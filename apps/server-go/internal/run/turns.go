// Per-turn drive: chain turns with staged-prompt drain (execute/nextTurn),
// build one agent per prompt (runOneTurn), and swap DefaultTools entries
// for per-run bound instances (replaceTool).
package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

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
