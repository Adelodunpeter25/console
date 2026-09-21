// Interactive decisions: tool permission approvals and ask-question
// answers. Port of apps/server/api/src/services/run/run-decisions.ts.
// Handlers broadcast the request on the run hub and block until the
// answer/approve route resolves them, the run aborts, or they time out.
package run

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// decisionTimeout mirrors the TS 10-minute decision timeout.
const decisionTimeout = 10 * time.Minute

type approvalResult struct {
	allow bool
	err   error
}

type pendingApproval struct {
	sessionID string
	ch        chan approvalResult
}

type questionResult struct {
	answer tools.AskAnswer
	err    error
}

type pendingQuestion struct {
	sessionID string
	ch        chan questionResult
}

// Decisions tracks in-flight permission/question requests per session.
// All methods are safe for concurrent use.
type Decisions struct {
	mu        sync.Mutex
	approvals map[string]pendingApproval
	questions map[string]pendingQuestion
	// Timeout bounds one decision wait (overridable in tests).
	Timeout time.Duration
	// Notify fires after a request broadcasts on the hub (attention
	// banners). Nil-safe when unset.
	Notify func(ctx context.Context, sessionID string, event loop.Event)
}

func newDecisions() *Decisions {
	return NewDecisions()
}

// NewDecisions creates a decision tracker (exported for tests).
func NewDecisions() *Decisions {
	return &Decisions{approvals: map[string]pendingApproval{}, questions: map[string]pendingQuestion{}, Timeout: decisionTimeout}
}

func (d *Decisions) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return decisionTimeout
}

// ApproverFor returns a loop.Approver that broadcasts permissionRequest on
// the hub and waits for the approve route.
func (d *Decisions) ApproverFor(sessionID string, hub *Hub) loop.Approver {
	return approverFunc(func(ctx context.Context, req permissions.Request) (bool, error) {
		ch := make(chan approvalResult, 1)
		d.mu.Lock()
		d.approvals[req.RequestID] = pendingApproval{sessionID: sessionID, ch: ch}
		notify := d.Notify
		d.mu.Unlock()
		event := loop.Event{Kind: loop.EventPermissionRequest, Permission: &req}
		hub.Broadcast(event)
		if notify != nil {
			notify(ctx, sessionID, event)
		}

		timer := time.NewTimer(d.timeout())
		defer timer.Stop()
		select {
		case res := <-ch:
			return res.allow, res.err
		case <-ctx.Done():
			d.removeApproval(req.RequestID)
			return false, ctx.Err()
		case <-timer.C:
			d.removeApproval(req.RequestID)
			return false, fmt.Errorf("Permission request timed out waiting for a decision.")
		}
	})
}

// AskHandlerFor returns a tools.AskHandler that broadcasts askQuestion on
// the hub and waits for the answer route.
func (d *Decisions) AskHandlerFor(sessionID string, hub *Hub) tools.AskHandler {
	return func(ctx context.Context, req tools.AskQuestionRequest) (tools.AskAnswer, error) {
		ch := make(chan questionResult, 1)
		d.mu.Lock()
		d.questions[req.RequestID] = pendingQuestion{sessionID: sessionID, ch: ch}
		notify := d.Notify
		d.mu.Unlock()
		event := loop.Event{Kind: loop.EventAskQuestion, Ask: &req}
		hub.Broadcast(event)
		if notify != nil {
			notify(ctx, sessionID, event)
		}

		timer := time.NewTimer(d.timeout())
		defer timer.Stop()
		select {
		case res := <-ch:
			return res.answer, res.err
		case <-ctx.Done():
			d.removeQuestion(req.RequestID)
			return tools.AskAnswer{}, ctx.Err()
		case <-timer.C:
			d.removeQuestion(req.RequestID)
			return tools.AskAnswer{}, fmt.Errorf("Question timed out waiting for a decision.")
		}
	}
}

// ApprovePermission resolves a pending approval. Returns false when no
// such request exists for the session.
func (d *Decisions) ApprovePermission(sessionID, requestID string, allow bool) bool {
	d.mu.Lock()
	pending, ok := d.approvals[requestID]
	if !ok || pending.sessionID != sessionID {
		d.mu.Unlock()
		return false
	}
	delete(d.approvals, requestID)
	d.mu.Unlock()
	pending.ch <- approvalResult{allow: allow}
	return true
}

// AnswerQuestion resolves a pending question. Returns false when no such
// request exists for the session.
func (d *Decisions) AnswerQuestion(sessionID, requestID string, answer tools.AskAnswer) bool {
	d.mu.Lock()
	pending, ok := d.questions[requestID]
	if !ok || pending.sessionID != sessionID {
		d.mu.Unlock()
		return false
	}
	delete(d.questions, requestID)
	d.mu.Unlock()
	pending.ch <- questionResult{answer: answer}
	return true
}

// RejectAllForSession fails every pending decision for a session (abort,
// run end). Other sessions are unaffected.
func (d *Decisions) RejectAllForSession(sessionID string, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, pending := range d.approvals {
		if pending.sessionID != sessionID {
			continue
		}
		delete(d.approvals, id)
		pending.ch <- approvalResult{err: fmt.Errorf("%s", reason)}
	}
	for id, pending := range d.questions {
		if pending.sessionID != sessionID {
			continue
		}
		delete(d.questions, id)
		pending.ch <- questionResult{err: fmt.Errorf("%s", reason)}
	}
}

func (d *Decisions) removeApproval(requestID string) {
	d.mu.Lock()
	delete(d.approvals, requestID)
	d.mu.Unlock()
}

func (d *Decisions) removeQuestion(requestID string) {
	d.mu.Lock()
	delete(d.questions, requestID)
	d.mu.Unlock()
}

// approverFunc adapts a function to loop.Approver.
type approverFunc func(ctx context.Context, req permissions.Request) (bool, error)

func (f approverFunc) Approve(ctx context.Context, req permissions.Request) (bool, error) {
	return f(ctx, req)
}
