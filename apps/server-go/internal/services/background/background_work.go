package background

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// Kind identifies the subsystem executing detached work.
type Kind string

const (
	KindShell    Kind = "shell"
	KindSubagent Kind = "subagent"
)

// Status is the durable lifecycle state exposed to callbacks and
// recovery callers.
type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

type EventType string

const (
	EventStarted   EventType = "backgroundWorkStarted"
	EventProgress  EventType = "backgroundWorkProgress"
	EventCompleted EventType = "backgroundWorkCompleted"
	EventFailed    EventType = "backgroundWorkFailed"
	EventCancelled EventType = "backgroundWorkCancelled"
)

// Work is a bounded, session-owned record. Backend-specific data
// belongs in Metadata; callers should use backend APIs for full output.
type Work struct {
	ID           string            `json:"id"`
	SessionID    string            `json:"sessionId"`
	ParentRunID  string            `json:"parentRunId,omitempty"`
	ParentCallID string            `json:"parentCallId,omitempty"`
	Kind         Kind              `json:"kind"`
	Status       Status            `json:"status"`
	CreatedAt    string            `json:"createdAt"`
	StartedAt    string            `json:"startedAt,omitempty"`
	FinishedAt   string            `json:"finishedAt,omitempty"`
	Summary      string            `json:"summary,omitempty"`
	Error        string            `json:"error,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Event is delivered for every lifecycle transition.
type Event struct {
	Type EventType `json:"type"`
	Work Work      `json:"work"`
}

// Listener receives events after they are recorded. It should
// be quick; listeners are called asynchronously so worker completion is never
// held up by a disconnected client.
type Listener func(Event)

type backgroundWorkRecord struct {
	work      Work
	cancel    func()
	delivered bool
}

// Manager owns detached work records and provides a small,
// backend-neutral lifecycle. Records remain available after completion for
// reconnect/recovery callers until the retention limit is reached.
type Manager struct {
	mu        sync.Mutex
	works     map[string]*backgroundWorkRecord
	listener  Listener
	maxRetain int
}

func NewManager() *Manager {
	return &Manager{works: make(map[string]*backgroundWorkRecord), maxRetain: 100}
}

func (m *Manager) SetListener(listener Listener) {
	m.mu.Lock()
	m.listener = listener
	m.mu.Unlock()
}

// Register creates a running record and emits its start event.
func (m *Manager) Register(opts Options) (Work, error) {
	if opts.SessionID == "" {
		return Work{}, fmt.Errorf("background work session id is required")
	}
	if opts.Kind == "" {
		return Work{}, fmt.Errorf("background work kind is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	work := Work{
		ID:           "work_" + utils.RandomID()[:16],
		SessionID:    opts.SessionID,
		ParentRunID:  opts.ParentRunID,
		ParentCallID: opts.ParentCallID,
		Kind:         opts.Kind,
		Status:       StatusRunning,
		CreatedAt:    now,
		StartedAt:    now,
		Metadata:     cloneMetadata(opts.Metadata),
	}
	m.mu.Lock()
	m.retainLocked()
	m.works[work.ID] = &backgroundWorkRecord{work: work}
	listener := m.listener
	m.mu.Unlock()
	m.emit(listener, Event{Type: EventStarted, Work: work})
	return work, nil
}

type Options struct {
	SessionID    string
	ParentRunID  string
	ParentCallID string
	Kind         Kind
	Metadata     map[string]string
}

// SetCancel attaches the backend cancellation operation. It is deliberately
// kept out of Work so wire clients cannot invoke arbitrary funcs.
func (m *Manager) SetCancel(workID string, cancel func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.works[workID]
	if !ok {
		return fmt.Errorf("background work %q not found", workID)
	}
	rec.cancel = cancel
	return nil
}

// Cancel requests cancellation and records the terminal state. The backend
// callback may race with its own exit; Complete is idempotent and wins once.
func (m *Manager) Cancel(workID, sessionID string) (Work, error) {
	m.mu.Lock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		m.mu.Unlock()
		return Work{}, err
	}
	cancel := rec.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return m.finish(workID, sessionID, StatusCancelled, "", "cancelled", nil)
}

func (m *Manager) Complete(workID, sessionID string, status Status, summary, failure string, metadata map[string]string) (Work, error) {
	return m.finish(workID, sessionID, status, summary, failure, metadata)
}

func (m *Manager) finish(workID, sessionID string, status Status, summary, failure string, metadata map[string]string) (Work, error) {
	m.mu.Lock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		m.mu.Unlock()
		return Work{}, err
	}
	if rec.work.Status != StatusRunning {
		work := cloneWork(rec.work)
		m.mu.Unlock()
		return work, nil
	}
	rec.work.Status = status
	rec.work.Summary = boundedText(summary, 2000)
	rec.work.Error = boundedText(failure, 2000)
	rec.work.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	for k, v := range metadata {
		if rec.work.Metadata == nil {
			rec.work.Metadata = map[string]string{}
		}
		rec.work.Metadata[k] = boundedText(v, 500)
	}
	work := cloneWork(rec.work)
	listener := m.listener
	m.mu.Unlock()

	typeName := EventCompleted
	switch status {
	case StatusFailed:
		typeName = EventFailed
	case StatusCancelled, StatusTimedOut:
		typeName = EventCancelled
	}
	m.emit(listener, Event{Type: typeName, Work: work})
	return work, nil
}

// CancelOwnedBy cancels every running work item owned by sessionID. It is
// used when a session run is aborted so detached shells and subagents do not
// survive the authorization boundary that created them.
func (m *Manager) CancelOwnedBy(sessionID string) {
	m.mu.Lock()
	ids := make([]string, 0)
	for id, rec := range m.works {
		if rec.work.SessionID == sessionID && rec.work.Status == StatusRunning {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_, _ = m.Cancel(id, sessionID)
	}
}

func (m *Manager) Get(workID, sessionID string) (Work, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		return Work{}, err
	}
	return cloneWork(rec.work), nil
}

func (m *Manager) List(sessionID string) []Work {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Work, 0, len(m.works))
	for _, rec := range m.works {
		if rec.work.SessionID == sessionID {
			out = append(out, cloneWork(rec.work))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// TakeCompletions returns terminal work that has not yet been delivered to the
// parent agent. Claiming is atomic, so a completion appears in at most one
// later parent turn even if two turns race to start.
func (m *Manager) TakeCompletions(sessionID string) []Work {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Work, 0)
	for _, rec := range m.works {
		if rec.work.SessionID != sessionID || rec.delivered || rec.work.Status == StatusRunning {
			continue
		}
		rec.delivered = true
		out = append(out, cloneWork(rec.work))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FinishedAt < out[j].FinishedAt })
	return out
}

// HasUndelivered reports whether the session has finished work the parent has
// not seen yet. Cancelled work is ignored: a kill or abort is already known to
// whoever requested it and must never wake an idle session.
func (m *Manager) HasUndelivered(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, rec := range m.works {
		if rec.work.SessionID != sessionID || rec.delivered {
			continue
		}
		switch rec.work.Status {
		case StatusCompleted, StatusFailed, StatusTimedOut:
			return true
		}
	}
	return false
}

func (m *Manager) ownedLocked(workID, sessionID string) (*backgroundWorkRecord, error) {
	rec, ok := m.works[workID]
	if !ok || rec.work.SessionID != sessionID {
		return nil, fmt.Errorf("background work %q not found or not owned by session", workID)
	}
	return rec, nil
}

func (m *Manager) retainLocked() {
	if len(m.works) < m.maxRetain {
		return
	}
	var oldestID string
	var oldest string
	for id, rec := range m.works {
		if rec.work.Status == StatusRunning {
			continue
		}
		if oldestID == "" || rec.work.CreatedAt < oldest {
			oldestID, oldest = id, rec.work.CreatedAt
		}
	}
	if oldestID != "" {
		delete(m.works, oldestID)
	}
}

func (m *Manager) emit(listener Listener, event Event) {
	if listener != nil {
		go listener(event)
	}
}

func cloneWork(work Work) Work {
	work.Metadata = cloneMetadata(work.Metadata)
	return work
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for k, v := range metadata {
		out[k] = v
	}
	return out
}

func boundedText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
