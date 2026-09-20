// Agent run execution service. Port of the run/agent-loop slice of
// apps/server/api/src/services/run.service.ts: active-run tracking with
// abort, a broadcast hub with replay for attaching clients, and one
// Agent drive per run. Queue/steer/approve/answer arrive in later slices.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

var (
	// ErrActive is returned when a run starts while one is already active.
	ErrActive = errors.New("session already has an active run")
	// ErrNoSession is returned when the session does not exist.
	ErrNoSession = errors.New("session not found")
)

// Attachment is a base64 image sent with a run prompt.
type Attachment struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// Prompt mirrors RunPromptDto: one user turn request.
type Prompt struct {
	Text         string
	ModelID      string
	Provider     string
	ApprovalMode string
	Thinking     string
	Attachments  []Attachment
}

// Terminal outcomes broadcast when a run settles.
const (
	OutcomeDone    = "done"
	OutcomeAborted = "aborted"
)

// Frame is one sequenced hub event for attaching clients.
type Frame struct {
	Seq   int64
	Event loop.Event
}

// Hub broadcasts run events to live subscribers and keeps a bounded ring
// for ?since= replay. Slow subscribers are evicted (they re-attach with
// since) so a stuck client never stalls the run.
type Hub struct {
	mu     sync.Mutex
	seq    int64
	buf    []Frame
	subs   map[string]chan Frame
	closed bool
	done   chan struct{}
	// Outcome is set by Close: "done" or "aborted".
	Outcome string
}

const hubBuffer = 500
const subBuffer = 256

// NewHub creates a broadcast hub (exported for tests and attach flows).
func NewHub() *Hub {
	return &Hub{subs: map[string]chan Frame{}, done: make(chan struct{})}
}

// Broadcast assigns the next sequence number and delivers to subscribers.
func (h *Hub) Broadcast(event loop.Event) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.seq++
	frame := Frame{Seq: h.seq, Event: event}
	h.buf = append(h.buf, frame)
	if len(h.buf) > hubBuffer {
		h.buf = h.buf[len(h.buf)-hubBuffer:]
	}
	for id, ch := range h.subs {
		select {
		case ch <- frame:
		default:
			delete(h.subs, id)
			close(ch)
		}
	}
	h.mu.Unlock()
}

// Subscribe registers a live subscriber. When since != nil the buffered
// frames newer than since are returned for replay first.
func (h *Hub) Subscribe(since *int64) (id string, ch <-chan Frame, replay []Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return "", nil, nil
	}
	id = newSubID()
	c := make(chan Frame, subBuffer)
	h.subs[id] = c
	if since != nil {
		for _, f := range h.buf {
			if f.Seq > *since {
				replay = append(replay, f)
			}
		}
	}
	return id, c, replay
}

// Unsubscribe removes a subscriber and drains its channel.
func (h *Hub) Unsubscribe(id string) {
	h.mu.Lock()
	if ch, ok := h.subs[id]; ok {
		delete(h.subs, id)
		close(ch)
	}
	h.mu.Unlock()
}

// Close terminates the hub with an outcome, waking settle waiters.
func (h *Hub) Close(outcome string) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	h.Outcome = outcome
	for id, ch := range h.subs {
		delete(h.subs, id)
		close(ch)
	}
	close(h.done)
	h.mu.Unlock()
}

// Done closes when the run settles.
func (h *Hub) Done() <-chan struct{} { return h.done }

func newSubID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "sub_" + hex.EncodeToString(b)
}

type activeRun struct {
	hub    *Hub
	cancel context.CancelFunc
	done   chan struct{}
}

// Service coordinates agent runs: one active run per session, abort, and
// hub lookup for attaching streams.
type Service struct {
	mu        sync.Mutex
	active    map[string]*activeRun
	sessions  *services.SessionService
	decisions *Decisions
	// Lookup resolves a provider id to a backend (overridable in tests).
	Lookup func(id string) (loop.Provider, error)
}

func NewService(sessions *services.SessionService) *Service {
	return &Service{active: map[string]*activeRun{}, sessions: sessions, decisions: newDecisions(), Lookup: providers.Lookup}
}

// IsActive reports whether the session has an in-flight run.
func (s *Service) IsActive(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[sessionID]
	return ok
}

// Hub returns the live hub for a session, or nil when no run is active.
func (s *Service) Hub(sessionID string) *Hub {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ar, ok := s.active[sessionID]; ok {
		return ar.hub
	}
	return nil
}

// StartRun validates, builds the agent, and drives the run in the
// background, returning the hub immediately for streaming.
func (s *Service) StartRun(sessionID string, dto Prompt) (*Hub, error) {
	s.mu.Lock()
	if _, ok := s.active[sessionID]; ok {
		s.mu.Unlock()
		return nil, ErrActive
	}
	s.mu.Unlock()

	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, ErrNoSession
	}
	header := loaded.Header

	providerID := dto.Provider
	if providerID == "" {
		providerID = header.Provider
	}
	modelID := dto.ModelID
	if modelID == "" {
		modelID = header.ModelID
	}
	approval := dto.ApprovalMode
	if approval == "" {
		approval = header.ApprovalMode
	}
	mode := permissions.Mode(approval)
	switch mode {
	case permissions.AlwaysAsk, permissions.AcceptEdits, permissions.PlanMode, permissions.FullAccess:
	default:
		mode = permissions.AlwaysAsk
	}

	provider, err := s.Lookup(providerID)
	if err != nil {
		return nil, err
	}

	history := decodeHistory(loaded.Messages)
	prompt := systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
		Cwd:          header.Cwd,
		Model:        modelID,
		ApprovalMode: systemprompt.ApprovalMode(mode),
	})

	hub := NewHub()
	askHandler := s.decisions.AskHandlerFor(sessionID, hub)
	toolList := make([]tools.Tool, 0, len(tools.DefaultTools()))
	for _, t := range tools.DefaultTools() {
		switch t.Name() {
		case "ask":
			toolList = append(toolList, tools.NewAskTool(askHandler))
		case "askMany":
			toolList = append(toolList, tools.NewAskManyTool(askHandler))
		default:
			toolList = append(toolList, t)
		}
	}
	registry := tools.NewRegistry(toolList...)
	executor := loop.NewExecutor(registry, mode, s.decisions.ApproverFor(sessionID, hub))
	agent := loop.New(provider, executor, s.sessions)
	agent.SystemPrompt = prompt.SystemPrompt
	agent.Model = modelID
	agent.CacheRetention = loop.CacheShort
	// Stable per conversation, scoped by provider+model so cached prefixes
	// stay valid (mirrors the TS cache-identity rotation rule).
	agent.ConversationID = fmt.Sprintf("%s:%s:%s", sessionID, providerID, modelID)
	agent.ThinkingLevel = dto.Thinking

	user := loop.UserMessage{Role: loop.RoleUser, Content: dto.Text}
	for _, a := range dto.Attachments {
		user.Attachments = append(user.Attachments, loop.ImageAttachment{Data: a.Data, MimeType: a.MimeType})
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.mu.Lock()
	// Re-check under lock: a concurrent StartRun may have won the race.
	if _, ok := s.active[sessionID]; ok {
		s.mu.Unlock()
		cancel()
		return nil, ErrActive
	}
	s.active[sessionID] = &activeRun{hub: hub, cancel: cancel, done: done}
	s.mu.Unlock()

	go s.execute(ctx, sessionID, agent, registry, history, user, hub, done)
	return hub, nil
}

func (s *Service) execute(ctx context.Context, sessionID string, agent *loop.Agent, registry *tools.Registry, history []any, user loop.UserMessage, hub *Hub, done chan struct{}) {
	defer close(done)
	defer func() {
		s.decisions.RejectAllForSession(sessionID, "Run ended")
		s.mu.Lock()
		delete(s.active, sessionID)
		s.mu.Unlock()
	}()

	events, err := agent.RunWithHistory(ctx, sessionID, history, user, registry.Definitions())
	if err != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		hub.Close(OutcomeDone)
		return
	}
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
			}
			break
		}
		hub.Broadcast(event)
	}
	if ctx.Err() != nil {
		hub.Close(OutcomeAborted)
	} else {
		hub.Close(OutcomeDone)
	}
}

// Abort cancels the active run. The run goroutine owns hub/session
// lifecycle, so the entry is removed when it settles — never here, or a
// second run could start on a still-draining session.
func (s *Service) Abort(sessionID string) bool {
	s.mu.Lock()
	ar, ok := s.active[sessionID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	ar.cancel()
	s.decisions.RejectAllForSession(sessionID, "Run aborted")
	return true
}

// ApprovePermission resolves a pending tool approval for a session.
func (s *Service) ApprovePermission(sessionID, requestID string, allow bool) bool {
	return s.decisions.ApprovePermission(sessionID, requestID, allow)
}

// AnswerQuestion resolves a pending ask-question for a session.
func (s *Service) AnswerQuestion(sessionID, requestID string, answer tools.AskAnswer) bool {
	return s.decisions.AnswerQuestion(sessionID, requestID, answer)
}
