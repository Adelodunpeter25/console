// Agent run execution service. Port of the run/agent-loop slice of
// apps/server/api/src/services/run.service.ts: active-run tracking with
// abort, turn chaining with staged-prompt drain, and one Agent drive per
// turn. Events flow through the Hub (hub.go); queue/steer decisions live
// in queue.go/decisions.go.
package run

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/memory"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
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
	// Lookup resolves a provider id to a backend (overridable in tests).
	Lookup func(id string) (loop.Provider, error)
}

func NewService(sessions *services.SessionService) *Service {
	s := &Service{active: map[string]*activeRun{}, pending: map[string]Prompt{}, sessions: sessions, decisions: newDecisions(), Lookup: providers.Lookup}
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
	return hub, nil
}

// execute drives one turn per loop iteration, draining a staged prompt as
// the next turn when the previous settled cleanly. A failed turn holds
// (not auto-runs, not drops) any staged prompt. One hub spans the whole
// chain so subscribers keep gap-free sequence numbers.
func (s *Service) execute(ctx context.Context, sessionID string, first Prompt, firstProvider loop.Provider, firstProviderID string, hub *Hub, done chan struct{}) {
	defer close(done)
	defer func() {
		s.decisions.RejectAllForSession(sessionID, "Run ended")
		s.mu.Lock()
		delete(s.active, sessionID)
		s.mu.Unlock()
	}()

	current, currentCtx := first, ctx
	currentProvider, currentProviderID := firstProvider, firstProviderID
	var runErr error
	for {
		turnErr := s.runOneTurn(currentCtx, sessionID, current, hub, &currentProvider, &currentProviderID)
		runErr = turnErr
		next, hasNext := s.nextTurn(currentCtx, turnErr, sessionID, hub)
		if !hasNext {
			if currentCtx.Err() != nil {
				hub.Close(OutcomeAborted)
			} else {
				hub.Close(OutcomeDone)
				if runErr == nil {
					s.notifyDone(sessionID)
				}
			}
			return
		}
		nextCtx, nextCancel := context.WithCancel(context.Background())
		s.mu.Lock()
		if ar, ok := s.active[sessionID]; ok {
			ar.cancel = nextCancel
		} else {
			nextCancel()
		}
		s.mu.Unlock()
		current, currentCtx = next, nextCtx
	}
}

// nextTurn decides whether a staged prompt drains as the next turn. A clean
// settle always drains; a canceled settle drains only when staged (steer:
// halt now, staged prompt runs next). Other errors hold the staged prompt
// so the user can steer, edit, or delete it.
func (s *Service) nextTurn(ctx context.Context, turnErr error, sessionID string, hub *Hub) (Prompt, bool) {
	if turnErr != nil && !errors.Is(turnErr, context.Canceled) {
		return Prompt{}, false
	}
	staged, ok := s.takeStaged(sessionID)
	if !ok {
		return Prompt{}, false
	}
	hub.Broadcast(loop.Event{Kind: loop.EventQueueUpdated})
	return staged, true
}

// replaceTool swaps a DefaultTools entry for a per-run bound instance
// (simulated subagent → loop-bound real one). Appends when absent.
func replaceTool(list []tools.Tool, name string, replacement tools.Tool) []tools.Tool {
	out := make([]tools.Tool, 0, len(list))
	replaced := false
	for _, t := range list {
		if t.Name() == name {
			if !replaced {
				out = append(out, replacement)
				replaced = true
			}
			continue
		}
		out = append(out, t)
	}
	if !replaced {
		out = append(out, replacement)
	}
	return out
}

// runOneTurn builds the agent for one prompt and pumps its events to the
// hub, returning the terminal error (nil on success). The provider instance
// is reused unless the prompt switches provider id.
func (s *Service) runOneTurn(ctx context.Context, sessionID string, dto Prompt, hub *Hub, current *loop.Provider, currentID *string) error {
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
	}
	if loaded == nil {
		err := ErrNoSession
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
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

	if providerID != *currentID {
		switched, err := s.Lookup(providerID)
		if err != nil {
			hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
			return err
		}
		*current = switched
		*currentID = providerID
	}
	provider := *current

	// Resolve the run model (catalog hit or synthetic fallback) for
	// thinking validation and the vision fallback below.
	model := s.resolveModel(providerID, modelID)
	effectiveThinking := dto.Thinking
	if effectiveThinking == "" {
		effectiveThinking = model.DefaultThinking
	}
	user := loop.UserMessage{Role: loop.RoleUser, Content: dto.Text}
	for _, a := range dto.Attachments {
		user.Attachments = append(user.Attachments, loop.ImageAttachment{Data: a.Data, MimeType: a.MimeType})
	}
	if err := roles.ValidateLevel(model, effectiveThinking); err != nil {
		// TS persists the user message before validation fails, so the
		// failed turn is visible in history.
		_ = s.sessions.AppendMessage(sessionID, userMessageRecord(user))
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
	}

	// Image attachments on a model without image support fall back to the
	// configured vision role model (TS runAgentStreamInternal parity).
	if len(dto.Attachments) > 0 && !model.SupportsImages {
		if vision, ok := s.resolveVision(model); ok {
			model = vision
			modelID, providerID = vision.ID, vision.Provider
			if switched, err := s.Lookup(providerID); err == nil {
				provider = switched
				*current, *currentID = switched, providerID
			}
		}
	}

	history := decodeHistory(loaded.Messages)
	prompt := systemprompt.BuildSystemPrompt(systemprompt.BuildOptions{
		Cwd:          header.Cwd,
		Model:        modelID,
		ApprovalMode: systemprompt.ApprovalMode(mode),
	})

	askHandler := s.decisions.AskHandlerFor(sessionID, hub)
	toolList := make([]tools.Tool, 0, len(tools.DefaultTools())+1)
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
	var projectID string
	if header.ProjectID != nil {
		projectID = *header.ProjectID
	}
	toolList = append(toolList, tools.NewMemoryTool(projectID, s.memoryRegistry()))
	toolList = replaceTool(toolList, "subagent", loop.NewSubagentTool(&loop.SubagentContext{
		Provider:     provider,
		Tools:        toolList,
		SystemPrompt: prompt.SystemPrompt,
		OnEvent:      hub.Broadcast,
	}))
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

	// First user turn on a placeholder title: generate one in the
	// background (TS session-title flow). Only applied if still generic.
	if titles.IsGenericTitle(header.Title) && len(history) == 0 {
		go s.generateTitle(sessionID, dto.Text, providerID, modelID, hub)
	}

	events, err := agent.RunWithHistory(ctx, sessionID, history, user, registry.Definitions())
	if err != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
	}
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
				s.notifyEvent(ctx, sessionID, loop.Event{Kind: loop.EventError, Text: err.Error()})
				return err
			}
			return nil
		}
		hub.Broadcast(event)
		s.notifyEvent(ctx, sessionID, event)
	}
}

// notifyEvent pushes an attention banner for questions, approvals, and
// errors (never for aborted runs).
func (s *Service) notifyEvent(ctx context.Context, sessionID string, event loop.Event) {
	bus := s.notifier()
	if bus == nil || !IsAttentionKind(event.Kind) || ctx.Err() != nil {
		return
	}
	title := ""
	if loaded, err := s.sessions.Load(sessionID, 0, 0); err == nil && loaded != nil {
		title = loaded.Header.Title
	}
	bus.Push(AttentionNotification(sessionID, event, title))
}

// notifyDone pushes the clean-completion banner with an excerpt.
func (s *Service) notifyDone(sessionID string) {
	bus := s.notifier()
	if bus == nil {
		return
	}
	title := ""
	if loaded, err := s.sessions.Load(sessionID, 0, 0); err == nil && loaded != nil {
		title = loaded.Header.Title
	}
	bus.Push(DoneNotification(sessionID, title, lastAssistantExcerpt(s.sessions, sessionID)))
}

// generateTitle resolves the session title off the critical path: LLM
// title first, truncation fallback on failure/empty. Applied + broadcast
// only while the stored title is still generic. Uses a fresh provider
// instance so generation never interferes with the running turn.
func (s *Service) generateTitle(sessionID, prompt, providerID, modelID string, hub *Hub) {
	// Titles use the configured smol role model (TS generateSessionTitle
	// parity), falling back to the run model.
	titleModel := roles.ResolveRoleModel(roles.Smol, s.resolveModel(providerID, modelID), s.roleRef(roles.Smol))
	title := ""
	if provider, err := s.Lookup(titleModel.Provider); err == nil {
		title = titles.Generate(context.Background(), provider, titleModel.ID, prompt)
	}
	if title == "" {
		title = titles.FallbackTitle(prompt)
	}
	loaded, err := s.sessions.Load(sessionID, 0, 0)
	if err != nil || loaded == nil || !titles.IsGenericTitle(loaded.Header.Title) {
		return
	}
	if err := s.sessions.UpdateTitle(sessionID, title); err != nil {
		return
	}
	hub.Broadcast(loop.Event{Kind: loop.EventSessionTitleUpdated, Title: title})
}

// resolveModel returns the catalog entry for a run model, synthesizing a
// fallback with inferred thinking levels for unknown ids (TS
// resolveRoleModel parity).
func (s *Service) resolveModel(providerID, modelID string) types.Model {
	if found, ok := providers.FindModel(providerID, modelID); ok {
		return found
	}
	levels, def := roles.InferThinkingLevels(providerID, modelID)
	return types.Model{
		ID: modelID, Provider: providerID, ContextWindow: 128_000,
		ThinkingLevels: levels, DefaultThinking: def,
	}
}

// resolveVision maps a model without image support to the configured
// vision role model when it does support images.
func (s *Service) resolveVision(model types.Model) (types.Model, bool) {
	vision := roles.ResolveRoleModel(roles.Vision, model, s.roleRef(roles.Vision))
	if !vision.SupportsImages {
		return model, false
	}
	return vision, true
}

// roleRef reads one configured model-role reference from settings.
func (s *Service) roleRef(role string) string {
	return services.NewSettingsService().Load().ModelRoles[role]
}

// userMessageRecord wraps a user message for storage (same shape the
// loop persists it in).
func userMessageRecord(user loop.UserMessage) types.AgentMessage {
	raw, err := json.Marshal(user)
	if err != nil {
		raw = []byte(`{"role":"user","content":"unserializable message"}`)
	}
	return types.AgentMessage{
		ID:   "msg_" + randomHex(16),
		Role: string(loop.RoleUser),
		Data: raw,
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
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
