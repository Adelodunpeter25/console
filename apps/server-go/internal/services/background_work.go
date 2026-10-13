package services

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// BackgroundWorkKind identifies the subsystem executing detached work.
type BackgroundWorkKind string

const (
	BackgroundWorkShell    BackgroundWorkKind = "shell"
	BackgroundWorkSubagent BackgroundWorkKind = "subagent"
)

// BackgroundWorkStatus is the durable lifecycle state exposed to callbacks and
// recovery callers.
type BackgroundWorkStatus string

const (
	BackgroundWorkRunning   BackgroundWorkStatus = "running"
	BackgroundWorkCompleted BackgroundWorkStatus = "completed"
	BackgroundWorkFailed    BackgroundWorkStatus = "failed"
	BackgroundWorkCancelled BackgroundWorkStatus = "cancelled"
	BackgroundWorkTimedOut  BackgroundWorkStatus = "timed_out"
)

type BackgroundWorkEventType string

const (
	BackgroundWorkStartedEvent   BackgroundWorkEventType = "backgroundWorkStarted"
	BackgroundWorkProgressEvent  BackgroundWorkEventType = "backgroundWorkProgress"
	BackgroundWorkCompletedEvent BackgroundWorkEventType = "backgroundWorkCompleted"
	BackgroundWorkFailedEvent    BackgroundWorkEventType = "backgroundWorkFailed"
	BackgroundWorkCancelledEvent BackgroundWorkEventType = "backgroundWorkCancelled"
)

// BackgroundWork is a bounded, session-owned record. Backend-specific data
// belongs in Metadata; callers should use backend APIs for full output.
type BackgroundWork struct {
	ID           string               `json:"id"`
	SessionID    string               `json:"sessionId"`
	ParentRunID  string               `json:"parentRunId,omitempty"`
	ParentCallID string               `json:"parentCallId,omitempty"`
	Kind         BackgroundWorkKind   `json:"kind"`
	Status       BackgroundWorkStatus `json:"status"`
	CreatedAt    string               `json:"createdAt"`
	StartedAt    string               `json:"startedAt,omitempty"`
	FinishedAt   string               `json:"finishedAt,omitempty"`
	Summary      string               `json:"summary,omitempty"`
	Error        string               `json:"error,omitempty"`
	Metadata     map[string]string    `json:"metadata,omitempty"`
}

// BackgroundWorkEvent is delivered for every lifecycle transition.
type BackgroundWorkEvent struct {
	Type BackgroundWorkEventType `json:"type"`
	Work BackgroundWork          `json:"work"`
}

// BackgroundWorkListener receives events after they are recorded. It should
// be quick; listeners are called asynchronously so worker completion is never
// held up by a disconnected client.
type BackgroundWorkListener func(BackgroundWorkEvent)

type backgroundWorkRecord struct {
	work      BackgroundWork
	cancel    func()
	delivered bool
}

// BackgroundWorkManager owns detached work records and provides a small,
// backend-neutral lifecycle. Records remain available after completion for
// reconnect/recovery callers until the retention limit is reached.
type BackgroundWorkManager struct {
	mu        sync.Mutex
	works     map[string]*backgroundWorkRecord
	listener  BackgroundWorkListener
	maxRetain int
}

func NewBackgroundWorkManager() *BackgroundWorkManager {
	return &BackgroundWorkManager{works: make(map[string]*backgroundWorkRecord), maxRetain: 100}
}

func (m *BackgroundWorkManager) SetListener(listener BackgroundWorkListener) {
	m.mu.Lock()
	m.listener = listener
	m.mu.Unlock()
}

// Register creates a running record and emits its start event.
func (m *BackgroundWorkManager) Register(opts BackgroundWorkOptions) (BackgroundWork, error) {
	if opts.SessionID == "" {
		return BackgroundWork{}, fmt.Errorf("background work session id is required")
	}
	if opts.Kind == "" {
		return BackgroundWork{}, fmt.Errorf("background work kind is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	work := BackgroundWork{
		ID:           "work_" + randomID()[:16],
		SessionID:    opts.SessionID,
		ParentRunID:  opts.ParentRunID,
		ParentCallID: opts.ParentCallID,
		Kind:         opts.Kind,
		Status:       BackgroundWorkRunning,
		CreatedAt:    now,
		StartedAt:    now,
		Metadata:     cloneMetadata(opts.Metadata),
	}
	m.mu.Lock()
	m.retainLocked()
	m.works[work.ID] = &backgroundWorkRecord{work: work}
	listener := m.listener
	m.mu.Unlock()
	m.emit(listener, BackgroundWorkEvent{Type: BackgroundWorkStartedEvent, Work: work})
	return work, nil
}

type BackgroundWorkOptions struct {
	SessionID    string
	ParentRunID  string
	ParentCallID string
	Kind         BackgroundWorkKind
	Metadata     map[string]string
}

// SetCancel attaches the backend cancellation operation. It is deliberately
// kept out of BackgroundWork so wire clients cannot invoke arbitrary funcs.
func (m *BackgroundWorkManager) SetCancel(workID string, cancel func()) error {
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
func (m *BackgroundWorkManager) Cancel(workID, sessionID string) (BackgroundWork, error) {
	m.mu.Lock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		m.mu.Unlock()
		return BackgroundWork{}, err
	}
	cancel := rec.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return m.finish(workID, sessionID, BackgroundWorkCancelled, "", "cancelled", nil)
}

func (m *BackgroundWorkManager) Complete(workID, sessionID string, status BackgroundWorkStatus, summary, failure string, metadata map[string]string) (BackgroundWork, error) {
	return m.finish(workID, sessionID, status, summary, failure, metadata)
}

func (m *BackgroundWorkManager) finish(workID, sessionID string, status BackgroundWorkStatus, summary, failure string, metadata map[string]string) (BackgroundWork, error) {
	m.mu.Lock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		m.mu.Unlock()
		return BackgroundWork{}, err
	}
	if rec.work.Status != BackgroundWorkRunning {
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

	typeName := BackgroundWorkCompletedEvent
	switch status {
	case BackgroundWorkFailed:
		typeName = BackgroundWorkFailedEvent
	case BackgroundWorkCancelled, BackgroundWorkTimedOut:
		typeName = BackgroundWorkCancelledEvent
	}
	m.emit(listener, BackgroundWorkEvent{Type: typeName, Work: work})
	return work, nil
}

// CancelOwnedBy cancels every running work item owned by sessionID. It is
// used when a session run is aborted so detached shells and subagents do not
// survive the authorization boundary that created them.
func (m *BackgroundWorkManager) CancelOwnedBy(sessionID string) {
	m.mu.Lock()
	ids := make([]string, 0)
	for id, rec := range m.works {
		if rec.work.SessionID == sessionID && rec.work.Status == BackgroundWorkRunning {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		_, _ = m.Cancel(id, sessionID)
	}
}

func (m *BackgroundWorkManager) Get(workID, sessionID string) (BackgroundWork, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, err := m.ownedLocked(workID, sessionID)
	if err != nil {
		return BackgroundWork{}, err
	}
	return cloneWork(rec.work), nil
}

func (m *BackgroundWorkManager) List(sessionID string) []BackgroundWork {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BackgroundWork, 0, len(m.works))
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
func (m *BackgroundWorkManager) TakeCompletions(sessionID string) []BackgroundWork {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]BackgroundWork, 0)
	for _, rec := range m.works {
		if rec.work.SessionID != sessionID || rec.delivered || rec.work.Status == BackgroundWorkRunning {
			continue
		}
		rec.delivered = true
		out = append(out, cloneWork(rec.work))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FinishedAt < out[j].FinishedAt })
	return out
}

func (m *BackgroundWorkManager) ownedLocked(workID, sessionID string) (*backgroundWorkRecord, error) {
	rec, ok := m.works[workID]
	if !ok || rec.work.SessionID != sessionID {
		return nil, fmt.Errorf("background work %q not found or not owned by session", workID)
	}
	return rec, nil
}

func (m *BackgroundWorkManager) retainLocked() {
	if len(m.works) < m.maxRetain {
		return
	}
	var oldestID string
	var oldest string
	for id, rec := range m.works {
		if rec.work.Status == BackgroundWorkRunning {
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

func (m *BackgroundWorkManager) emit(listener BackgroundWorkListener, event BackgroundWorkEvent) {
	if listener != nil {
		go listener(event)
	}
}

func cloneWork(work BackgroundWork) BackgroundWork {
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
