// Agent run execution service: active-run tracking, abort, and hub
// lookup. Turn chaining lives in turns.go, queue/steer in queue.go,
// decisions in decisions.go, events in hub.go, notifications in notify.go.
package run

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
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
	ContextFiles []string
	ModelID      string
	Provider     string
	ApprovalMode string
	Thinking     string
	Attachments  []Attachment
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
	pending   map[string]Prompt
	sessions  *services.SessionService
	decisions *Decisions
	notify    *services.NotificationService
	memories  *memory.Registry
	bashJobs  *services.BashJobManager
	ports     *services.PortRegistry
	scripts   *services.ProjectScriptsService
	prompts   promptCache
	// snapshots holds pre-write file content for whole-file overwrite tools
	// (write_file, batchWrite), keyed by session then path. Populated just
	// before such a tool is dispatched and consulted when its result is
	// recorded, since by then the tool has already written the file. Dropped
	// when the run settles.
	snapshots *writeSnapshots
	// Lookup resolves a provider id to a backend (overridable in tests).
	Lookup func(id string) (loop.Provider, error)
	// WatchdogTimeout overrides defaultWatchdogTimeout. Zero uses the
	// default; negative disables the watchdog (tests only).
	WatchdogTimeout time.Duration
}

func NewService(sessions *services.SessionService) *Service {
	s := &Service{active: map[string]*activeRun{}, pending: map[string]Prompt{}, sessions: sessions, decisions: newDecisions(), Lookup: providers.Lookup, snapshots: newWriteSnapshots()}
	s.decisions.Notify = func(ctx context.Context, sessionID string, event loop.Event) {
		s.notifyEvent(ctx, sessionID, event)
	}
	return s
}

// SetNotifications attaches the bus for attention/done banners (nil-safe
// when unset).
func (s *Service) SetNotifications(n *services.NotificationService) {
	s.mu.Lock()
	s.notify = n
	s.mu.Unlock()
}

// SetMemories attaches the memory registry backing the per-run memory
// tool (nil-safe when unset: the tool reports unavailable).
func (s *Service) SetMemories(r *memory.Registry) {
	s.mu.Lock()
	s.memories = r
	s.mu.Unlock()
}

func (s *Service) memoryRegistry() *memory.Registry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memories
}

// SetBashJobs attaches the background-job manager backing the per-run
// bash/bashJob tools (nil-safe when unset: background mode reports
// unavailable, matching the unbound DefaultTools() instances).
func (s *Service) SetBashJobs(j *services.BashJobManager) {
	s.mu.Lock()
	s.bashJobs = j
	s.mu.Unlock()
}

func (s *Service) bashJobManager() *services.BashJobManager {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bashJobs
}

// SetPorts attaches the port registry backing the per-run ports tool.
func (s *Service) SetPorts(p *services.PortRegistry) {
	s.mu.Lock()
	s.ports = p
	s.mu.Unlock()
}

func (s *Service) portRegistry() *services.PortRegistry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ports
}

// SetProjectScripts attaches the scripts service backing the per-run project_scripts tool.
func (s *Service) SetProjectScripts(scripts *services.ProjectScriptsService) {
	s.mu.Lock()
	s.scripts = scripts
	s.mu.Unlock()
}

func (s *Service) projectScriptsService() *services.ProjectScriptsService {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scripts
}

func (s *Service) notifier() *services.NotificationService {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notify
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

// StartRun validates the first turn and drives the run chain in the
// background, returning the hub immediately for streaming. Staged prompts
// drain as further turns on the same hub (see execute).
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
	// Fail fast on an unknown provider before streaming starts. The
	// validated instance is reused for the first turn; later turns
	// re-resolve only when the staged prompt switches provider.
	providerID := dto.Provider
	if providerID == "" {
		providerID = loaded.Header.Provider
	}
	firstProvider, err := s.Lookup(providerID)
	if err != nil {
		return nil, err
	}

	hub := NewHub()
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

	go s.execute(ctx, sessionID, dto, firstProvider, providerID, hub, done)
	go s.watchRun(sessionID, hub, done)
	return hub, nil
}

// Abort cancels the active run. The run goroutine owns hub/session
// lifecycle, so the entry is removed when it settles — never here, or a
// second run could start on a still-draining session. Stop means stop
// everything: any background bash jobs the session started are killed too,
// not just the in-flight turn.
func (s *Service) Abort(sessionID string) bool {
	if !s.cancelActive(sessionID) {
		return false
	}
	s.decisions.RejectAllForSession(sessionID, "Run aborted")
	if jobs := s.bashJobManager(); jobs != nil {
		jobs.KillOwnedBy(sessionID)
	}
	slog.Info("run aborted", "session", sessionID)
	return true
}

// AbortAll cancels every active run (server shutdown). Best-effort: it
// signals cancellation and returns immediately without waiting for the
// runs to settle.
func (s *Service) AbortAll() {
	s.mu.Lock()
	ids := make([]string, 0, len(s.active))
	for id := range s.active {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.Abort(id)
	}
}

// SetDecisionTimeout overrides how long question/approval waits block
// (tests; zero restores the default).
func (s *Service) SetDecisionTimeout(d time.Duration) {
	s.decisions.mu.Lock()
	s.decisions.Timeout = d
	s.decisions.mu.Unlock()
}

// ApprovePermission resolves a pending tool approval for a session.
func (s *Service) ApprovePermission(sessionID, requestID string, allow bool) bool {
	return s.decisions.ApprovePermission(sessionID, requestID, allow)
}

// AnswerQuestion resolves a pending ask-question for a session.
func (s *Service) AnswerQuestion(sessionID, requestID string, answer tools.AskAnswer) bool {
	return s.decisions.AnswerQuestion(sessionID, requestID, answer)
}

// ResolveBrowserAction resolves a pending browser action for a session.
func (s *Service) ResolveBrowserAction(sessionID, requestID string, result tools.BrowserActionResult) bool {
	return s.decisions.ResolveBrowserAction(sessionID, requestID, result)
}
