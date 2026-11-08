// Interactive decisions: ask-question answers and browser actions.
// Handlers broadcast the request on the run hub and block until the
// answer route resolves them, the run aborts, or they time out.
package run

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// decisionTimeout is how long a decision waits before timing out.
const decisionTimeout = 10 * time.Minute

type questionResult struct {
	answer tools.AskAnswer
	err    error
}

type pendingQuestion struct {
	sessionID string
	ch        chan questionResult
}

type browserActionResult struct {
	result tools.BrowserActionResult
	err    error
}

type pendingBrowserAction struct {
	sessionID string
	ch        chan browserActionResult
}

// Decisions tracks in-flight question/browser requests per session.
// All methods are safe for concurrent use.
type Decisions struct {
	mu             sync.Mutex
	questions      map[string]pendingQuestion
	browserActions map[string]pendingBrowserAction
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
	return &Decisions{
		questions:      map[string]pendingQuestion{},
		browserActions: map[string]pendingBrowserAction{},
		Timeout:        decisionTimeout,
	}
}

func (d *Decisions) timeout() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Timeout > 0 {
		return d.Timeout
	}
	return decisionTimeout
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

// BrowserHandlerFor returns a tools.BrowserHandler that broadcasts browserAction on
// the hub and waits for the desktop client to respond via POST /api/sessions/:id/browser-action.
func (d *Decisions) BrowserHandlerFor(sessionID string, hub *Hub) tools.BrowserHandler {
	return func(ctx context.Context, req tools.BrowserActionRequest) (tools.BrowserActionResult, error) {
		ch := make(chan browserActionResult, 1)
		d.mu.Lock()
		d.browserActions[req.RequestID] = pendingBrowserAction{sessionID: sessionID, ch: ch}
		notify := d.Notify
		d.mu.Unlock()
		event := loop.Event{Kind: loop.EventBrowserAction, Browser: &req}
		hub.Broadcast(event)
		if notify != nil {
			notify(ctx, sessionID, event)
		}

		timer := time.NewTimer(d.timeout())
		defer timer.Stop()
		select {
		case res := <-ch:
			return res.result, res.err
		case <-ctx.Done():
			d.removeBrowserAction(req.RequestID)
			return tools.BrowserActionResult{}, ctx.Err()
		case <-timer.C:
			d.removeBrowserAction(req.RequestID)
			return tools.BrowserActionResult{}, fmt.Errorf("Browser action timed out waiting for desktop client.")
		}
	}
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

// ResolveBrowserAction resolves a pending browser action. Returns false when
// no such request exists for the session.
func (d *Decisions) ResolveBrowserAction(sessionID, requestID string, result tools.BrowserActionResult) bool {
	d.mu.Lock()
	pending, ok := d.browserActions[requestID]
	if !ok || pending.sessionID != sessionID {
		d.mu.Unlock()
		return false
	}
	delete(d.browserActions, requestID)
	d.mu.Unlock()
	pending.ch <- browserActionResult{result: result}
	return true
}

// RejectAllForSession fails every pending decision for a session (abort,
// run end). Other sessions are unaffected.
func (d *Decisions) RejectAllForSession(sessionID string, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, pending := range d.questions {
		if pending.sessionID != sessionID {
			continue
		}
		delete(d.questions, id)
		pending.ch <- questionResult{err: fmt.Errorf("%s", reason)}
	}
	for id, pending := range d.browserActions {
		if pending.sessionID != sessionID {
			continue
		}
		delete(d.browserActions, id)
		pending.ch <- browserActionResult{err: fmt.Errorf("%s", reason)}
	}
}

func (d *Decisions) removeQuestion(requestID string) {
	d.mu.Lock()
	delete(d.questions, requestID)
	d.mu.Unlock()
}

func (d *Decisions) removeBrowserAction(requestID string) {
	d.mu.Lock()
	delete(d.browserActions, requestID)
	d.mu.Unlock()
}
