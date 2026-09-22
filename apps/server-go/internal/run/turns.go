// Per-turn drive: chain turns with staged-prompt drain (execute/nextTurn),
// build one agent per prompt (runOneTurn), and swap DefaultTools entries
// for per-run bound instances (replaceTool).
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
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
	hub.Broadcast(loop.Event{Kind: loop.EventSessionStart})
	var runErr error
	for {
		turnErr := s.runOneTurn(currentCtx, sessionID, current, hub, &currentProvider, &currentProviderID)
		runErr = turnErr
		next, hasNext := s.nextTurn(currentCtx, turnErr, sessionID, hub)
		if !hasNext {
			// Terminal frame mirrors the TS finally: sessionEnd always
			// closes the wire stream, on done and on abort alike. A finished
			// (all-completed) todo list is cleared first so the next run
			// starts fresh, with an empty todoUpdate so the card clears too.
			if todos, err := s.sessions.GetSessionTodos(sessionID); err == nil && len(todos) > 0 {
				if err := s.sessions.ClearCompletedTodos(sessionID); err == nil {
					if cleared, err := s.sessions.GetSessionTodos(sessionID); err == nil && len(cleared) == 0 {
						hub.Broadcast(loop.Event{Kind: loop.EventTodoUpdate, Items: []types.TodoItem{}, Action: "updated"})
					}
				}
			}
			hub.Broadcast(loop.Event{Kind: loop.EventSessionEnd})
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
	user := loop.UserMessage{
		Role:         loop.RoleUser,
		Content:      dto.Text,
		ContextFiles: dto.ContextFiles,
	}
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
	onTodoUpdate := func(items []types.TodoItem, action string) {
		hub.Broadcast(loop.Event{Kind: loop.EventTodoUpdate, Items: items, Action: action})
	}
	toolList := make([]tools.Tool, 0, len(tools.DefaultTools())+1)
	for _, t := range tools.DefaultTools() {
		switch t.Name() {
		case "todo":
			toolList = append(toolList, tools.NewTodoToolWithUpdate(sessionID, s.sessions, onTodoUpdate))
		case "ask":
			toolList = append(toolList, tools.NewAskTool(askHandler))
		case "askMany":
			toolList = append(toolList, tools.NewAskManyTool(askHandler))
		case "bash":
			toolList = append(toolList, tools.NewBashTool(s.bashJobManager(), sessionID))
		case "bashJob":
			toolList = append(toolList, tools.NewBashJobTool(s.bashJobManager(), sessionID))
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
	agent.Compaction = s.compactionHooks(sessionID, model, prompt.SystemPrompt, registry.Definitions())

	hub.Broadcast(loop.Event{Kind: loop.EventTurnStart, Text: dto.Text})

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
	// Translate the flat provider stream into the TS/desktop wire
	// vocabulary: model-stream triplets bracketed by turn ids, tool phases
	// bracketed by execution start/end. Raw provider kinds (text/thinking/
	// toolCall/usage/toolResult/turnDone) never reach subscribers.
	turner := newTurnTranslator(hub)
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

		// Track file changes on tool execution results
		if event.Kind == loop.EventToolResult && event.Result != nil {
			turnIndex := len(history) // Approximate turn index from history length
			var args map[string]any
			if len(event.Result.Args) > 0 {
				_ = json.Unmarshal(event.Result.Args, &args)
			}
			_ = ExtractAndRecordFileChange(s.sessions, sessionID, event.Result.ToolName, args, event.Result.IsError, turnIndex)
		}

		turner.translate(event)
		s.notifyEvent(ctx, sessionID, event)
	}
}

// turnTranslator converts one agent run's flat provider events into the
// TS wire structure: model-stream parts pass through (turn triplets arrive
// bracketed from the loop with canonical snapshots), tool phases are
// bracketed by execution start/end around results.
type turnTranslator struct {
	hub      *Hub
	calls    []tools.ToolCall
	results  []tools.ToolResult
	toolOpen bool
}

func newTurnTranslator(hub *Hub) *turnTranslator {
	return &turnTranslator{hub: hub}
}

func (t *turnTranslator) flushTools() {
	if t.toolOpen {
		t.hub.Broadcast(loop.Event{Kind: loop.EventToolExecutionEnd, Results: t.results})
		t.calls = nil
		t.results = nil
		t.toolOpen = false
	}
}

func (t *turnTranslator) translate(event loop.Event) {
	switch event.Kind {
	case loop.EventText:
		t.hub.Broadcast(loop.Event{
			Kind: loop.EventModelStreamPart,
			Part: map[string]any{"text": event.Text},
		})
	case loop.EventThinking:
		t.hub.Broadcast(loop.Event{
			Kind: loop.EventModelStreamPart,
			Part: map[string]any{"thinking": event.Text},
		})
	case loop.EventToolCall:
		if event.Call == nil {
			return
		}
		t.hub.Broadcast(loop.Event{
			Kind: loop.EventModelStreamPart,
			Part: map[string]any{"toolCall": map[string]any{
				"id": event.Call.ID, "name": event.Call.Name,
			}},
		})
		t.calls = append(t.calls, *event.Call)
	case loop.EventToolResult:
		if event.Result == nil {
			return
		}
		if !t.toolOpen {
			t.hub.Broadcast(loop.Event{Kind: loop.EventToolExecutionStart, Calls: t.calls})
			t.toolOpen = true
		}
		t.hub.Broadcast(loop.Event{Kind: loop.EventToolExecutionResult, Result: event.Result})
		t.results = append(t.results, *event.Result)
	case loop.EventModelStreamEnd, loop.EventTurnEnd:
		t.flushTools()
		t.hub.Broadcast(event)
	case loop.EventModelStreamStart:
		// A new provider turn closes any dangling tool phase first,
		// keeping execution-end strictly before the next stream start.
		t.flushTools()
		t.hub.Broadcast(event)
	case loop.EventUsage, loop.EventTurnDone:
		// Token accounting and the terminal marker stay internal; TS does
		// not stream them and the desktop cannot parse them.
	default:
		t.hub.Broadcast(event)
	}
}
