// Per-turn drive: chain turns with staged-prompt drain (execute/nextTurn),
// build one agent per prompt (runOneTurn), and swap DefaultTools entries
// for per-run bound instances (replaceTool).
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/mcp"
	"log/slog"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/roles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/systemprompt"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/titles"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// setStatus persists the indexed session status that session lists and the
// mobile home badge read back. Best-effort: a status write must never fail a
// run, and a stale value is repaired by the settle-on-read path in
// routes/sessions.go when the run is no longer active.
func (s *Service) setStatus(sessionID, status string) {
	if s.sessions == nil {
		return
	}
	if err := s.sessions.UpdateStatus(sessionID, status); err != nil {
		slog.Warn("run: session status update failed", "session", sessionID, "status", status, "error", err)
	}
}

// execute drives one turn per loop iteration, draining a staged prompt as
// the next turn when the previous settled cleanly. A failed turn holds
// (not auto-runs, not drops) any staged prompt. One hub spans the whole
// chain so subscribers keep gap-free sequence numbers.
func (s *Service) execute(ctx context.Context, sessionID string, first Prompt, firstProvider loop.Provider, firstProviderID string, hub *Hub, done chan struct{}) {
	defer close(done)
	defer func() {
		// Pre-write snapshots are only meaningful for the run that captured
		// them; release them with the run so the map cannot grow unbounded
		// across a long-lived server.
		s.snapshots.drop(sessionID)
		// Only tear down state this run still owns: after a watchdog
		// force-settle a newer run may hold the slot and its decisions.
		s.mu.Lock()
		ar, owned := s.active[sessionID]
		owned = owned && ar.hub == hub
		if owned {
			delete(s.active, sessionID)
		}
		s.mu.Unlock()
		if owned {
			s.decisions.RejectAllForSession(sessionID, "Run ended")
		}
	}()

	current, currentCtx := first, ctx
	currentProvider, currentProviderID := firstProvider, firstProviderID
	// Mark working once per run, not per turn: a drained staged prompt keeps
	// the same run (and therefore the same status) across its turns.
	s.setStatus(sessionID, "working")
	hub.Broadcast(loop.Event{Kind: loop.EventSessionStart})
	var runErr error
	for {
		turnErr := s.runOneTurn(currentCtx, sessionID, current, hub, &currentProvider, &currentProviderID)
		runErr = turnErr
		if turnErr != nil && !errors.Is(turnErr, context.Canceled) {
			slog.Warn("run turn error", "session", sessionID, "error", turnErr)
		}
		next, hasNext := s.nextTurn(currentCtx, turnErr, sessionID, hub)
		if !hasNext {
			// Terminal frame: sessionEnd always
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
				// Abort/steer is a user action, not a failure: settle done
				// never needs_attention.
				s.setStatus(sessionID, "done")
				hub.Close(OutcomeAborted)
			} else {
				if runErr == nil {
					s.setStatus(sessionID, "done")
				} else {
					s.setStatus(sessionID, "needs_attention")
				}
				hub.Close(OutcomeDone)
				if runErr == nil {
					s.notifyDone(sessionID)
				}
			}
			return
		}
		nextCtx, nextCancel := context.WithCancel(context.Background())
		s.mu.Lock()
		if ar, ok := s.active[sessionID]; ok && ar.hub == hub {
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

// countUserMessages returns how many user messages a decoded history holds,
// which is the zero-based index of the next turn.
func countUserMessages(history []any) int {
	n := 0
	for _, msg := range history {
		if _, ok := msg.(loop.UserMessage); ok {
			n++
		}
	}
	return n
}

// writePaths returns the files a tool call is about to modify. Each gets a
// baseline snapshot on its first touch in the turn so the recorded change
// is the net effect of the whole turn; read-only tools touch nothing.
//
// Unparseable or empty argument payloads yield no paths, which simply means
// the later recording step falls back to argument-only information.
func writePaths(call tools.ToolCall) []string {
	switch call.Name {
	case "writeFile", "write_file", "batchWrite", "batch_write",
		"editFile", "edit_file", "replace_file_content":
	default:
		return nil
	}
	if len(call.Arguments) == 0 {
		return nil
	}
	// Both tools describe their targets the same way: a "path" for
	// write_file, or "files"[].path for batchWrite. Read both shapes so one
	// unmarshal serves either tool.
	var in struct {
		Path       string `json:"path"`
		TargetFile string `json:"targetFile"`
		Files      []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return nil
	}
	var paths []string
	if in.Path != "" {
		paths = append(paths, in.Path)
	} else if in.TargetFile != "" {
		paths = append(paths, in.TargetFile)
	}
	for _, f := range in.Files {
		if f.Path != "" {
			paths = append(paths, f.Path)
		}
	}
	return paths
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

	// /computer-use activates computer use for the session. The model has no
	// prior knowledge of it, so the command is matched here rather than
	// discovered. The message itself is never rewritten: what the user typed
	// is what gets persisted and displayed, and the skill instructions travel
	// in setup instead (sent every turn, never saved).
	if _, computerInvoked := cua.SplitInvocation(dto.Text); computerInvoked {
		s.computerUse.mark(sessionID)
	}

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
	model := s.resolveModel(ctx, providerID, modelID)
	effectiveThinking := dto.Thinking
	// OpenCode models are cataloged without thinking levels unless their family
	// has a known effort vocabulary. A session can carry a stale level after
	// switching providers, so discard it instead of failing the run or sending
	// reasoning controls to a model that does not declare support. Models that
	// do declare levels fall through to normal validation below.
	if model.Provider == "opencode" && len(model.ThinkingLevels) == 0 {
		effectiveThinking = ""
	} else if effectiveThinking == "" {
		effectiveThinking = model.DefaultThinking
	}
	user := loop.UserMessage{
		Role:         loop.RoleUser,
		Content:      dto.Text,
		ContextFiles: dto.ContextFiles,
		Annotations:  dto.Annotations,
	}
	for _, a := range dto.Attachments {
		user.Attachments = append(user.Attachments, loop.ImageAttachment{Data: a.Data, MimeType: a.MimeType})
	}
	if err := roles.ValidateLevel(model, effectiveThinking); err != nil {
		// Persist the user message even though validation fails, so the
		// failed turn is visible in history.
		_ = s.sessions.AppendMessage(sessionID, userMessageRecord(user))
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
	}

	// Image attachments on a model without image support fall back to the
	// configured vision role model.
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

	history := DecodeHistory(loaded.Messages)
	// A turn is one user message: its index is how many user messages the
	// session already holds (compaction never rewrites stored history).
	turn := turnRef{Index: countUserMessages(history), UserMessageID: "msg_" + randomHex(16)}
	// File baselines are per turn: the first touch of a path in this turn
	// captures its "before" side, so repeated edits record the net change.
	s.snapshots.drop(sessionID)
	prompt := s.prompts.get(sessionID, systemprompt.BuildOptions{
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
		case "ports":
			var pid string
			if header.ProjectID != nil {
				pid = *header.ProjectID
			}
			toolList = append(toolList, tools.NewPortsTool(pid, s.portRegistry()))
		case "project_scripts":
			var pid string
			if header.ProjectID != nil {
				pid = *header.ProjectID
			}
			toolList = append(toolList, tools.NewProjectScriptsTool(pid, sessionScopedScripts{svc: s.projectScriptsService(), cwd: header.Cwd}))
		case "browser":
			toolList = append(toolList, tools.NewBrowserTool(s.decisions.BrowserHandlerFor(sessionID, hub)))
		default:
			toolList = append(toolList, t)
		}
	}
	var projectID string
	if header.ProjectID != nil {
		projectID = *header.ProjectID
	}
	toolList = append(toolList, tools.NewMemoryTool(projectID, s.memoryRegistry()))
	usage := &loop.UsageTracker{}
	setup := prompt.Setup
	mcpManager := s.mcpManager()
	if mcpManager != nil && len(mcp.EnabledServers(mcpManager)) > 0 {
		setup = strings.TrimSpace(setup + "\n\n" + mcpSetupHint)
	}
	// Computer-use instructions ride setup rather than the user message: setup
	// is sent every turn but never persisted, so the chat history (and the
	// bubbles rendered from it) keeps exactly what the user typed.
	if s.computerUse.isActive(sessionID) {
		setup = strings.TrimSpace(setup + "\n\n" + cua.SkillText())
	}
	var registry *tools.Registry
	toolList = replaceTool(toolList, "subagent", loop.NewSubagentTool(&loop.SubagentContext{
		Provider: provider,
		Model:    modelID,
		Tools:    toolList,
		// Subagents start from the parent's current tools, so MCP groups the
		// parent already loaded carry over, and get their own loadTools.
		ToolSource: func() []tools.Tool { return registry.Tools() },
		NestedTools: func(nested *tools.Registry) {
			if mcpManager != nil {
				if lt := mcp.NewLoadToolsTool(mcpManager, nested); lt != nil {
					nested.Add(lt)
				}
			}
		},
		SystemPrompt: prompt.StableSystem,
		Setup:        setup,
		// A subagent inherits the parent run's mode: it holds the same tools,
		// so it must hold the same authority over them.
		ApprovalMode: mode,
		OnEvent:      hub.Broadcast,
		Usage:        usage,
	}))
	registry = tools.NewRegistry(toolList...)
	// MCP servers load lazily: only a small loadTools entry is in the list
	// until the model asks for a group, which then appends that server's
	// tools (visible from the next turn via agent.ToolDefs). Groups loaded
	// earlier in this session are restored up front, in load order.
	if mcpManager != nil {
		if lt := mcp.NewLoadToolsTool(mcpManager, registry, mcp.LoadToolsOptions{
			OnLoad: func(group string) { s.groups.add(sessionID, group) },
		}); lt != nil {
			registry.Add(lt)
			for _, group := range s.groups.list(sessionID) {
				if _, _, err := mcp.LoadGroup(ctx, mcpManager, registry, group); err != nil {
					slog.Warn("restore tool group failed", "session", sessionID, "group", group, "error", err)
				}
			}
		}
	}
	// Computer use restores the same way: sessions that invoked /computer-use
	// keep the two tools from the next turn on, appended after everything
	// else so the provider's cached prefix stays stable. With no driver the
	// load fails honestly and the model is told, so it reports unavailability
	// instead of calling into the void.
	if s.computerUse.isActive(sessionID) {
		loader := cua.NewLoader(s.cuaManager, sessionID)
		if _, err := loader.Load(registry); err != nil {
			user.Content += "\n\nComputer use is unavailable: " + err.Error()
		}
	}
	executor := loop.NewExecutor(registry, mode, s.decisions.ApproverFor(sessionID, hub))
	// Capture the pre-write content of every file a whole-file overwrite
	// tool is about to touch, so the change diff recorded from the result
	// event (which fires after the write) still has a real "before" side.
	executor.OnBeforeExecute = func(call tools.ToolCall, _ tools.Tool) {
		s.snapshots.SnapshotWritePaths(sessionID, writePaths(call))
	}
	agent := loop.New(provider, executor, s.sessions)
	agent.Usage = usage
	agent.UserMessageID = turn.UserMessageID
	agent.ToolDefs = registry.Definitions
	agent.SystemPrompt = prompt.StableSystem
	agent.Setup = setup
	agent.SystemSections = systemSections(prompt.Sections)
	agent.Model = modelID
	agent.CacheRetention = loop.CacheShort
	// Stable per conversation, scoped by provider+model so cached prefixes
	// stay valid.
	agent.ConversationID = fmt.Sprintf("%s:%s:%s", sessionID, providerID, modelID)
	agent.ThinkingLevel = effectiveThinking
	agent.Compaction = s.compactionHooks(sessionID, model, prompt.SystemPrompt, registry.Definitions())

	hub.Broadcast(loop.Event{Kind: loop.EventTurnStart, Text: dto.Text})

	// First user turn on a placeholder title: generate one in the
	// background. Only applied if still generic.
	if titles.IsGenericTitle(header.Title) && len(history) == 0 {
		go s.generateTitle(sessionID, dto.Text, providerID, modelID, hub)
	}

	events, err := agent.RunWithHistory(ctx, sessionID, history, user, registry.Definitions())
	if err != nil {
		hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
		return err
	}
	// Translate the flat provider stream into the desktop wire
	// vocabulary: model-stream triplets bracketed by turn ids, tool phases
	// bracketed by execution start/end. Raw provider kinds (text/thinking/
	// toolCall/usage/toolResult/turnDone) never reach subscribers.
	turner := newTurnTranslator(hub)
	defer func() {
		total := usage.Snapshot().Total()
		slog.Info("run usage", "session", sessionID, "provider", providerID, "model", modelID,
			"turns", total.Turns, "input", total.Input, "cacheRead", total.CacheRead,
			"cacheWrite", total.CacheWrite, "output", total.Output, "coldMisses", total.ColdMisses)
	}()
	for {
		event, err, ok := events.Next()
		if !ok {
			turner.flushTools()
			if err != nil {
				// Abort/steer is a user action, not a failure: no error
				// frame or attention banner.
				if errors.Is(err, context.Canceled) || ctx.Err() != nil {
					s.broadcastContextUsage(ctx, sessionID, hub)
					return context.Canceled
				}
				hub.Broadcast(loop.Event{Kind: loop.EventError, Text: err.Error()})
				s.notifyEvent(ctx, sessionID, loop.Event{Kind: loop.EventError, Text: err.Error()})
				s.broadcastContextUsage(ctx, sessionID, hub)
				return err
			}
			s.broadcastContextUsage(ctx, sessionID, hub)
			return nil
		}

		// Track file changes on tool execution results
		if event.Kind == loop.EventToolResult && event.Result != nil {
			var args map[string]any
			if len(event.Result.Args) > 0 {
				_ = json.Unmarshal(event.Result.Args, &args)
			}
			_ = s.generateAndRecordFileChange(s.snapshots, sessionID, event.Result.ToolName, args, event.Result.IsError, turn)
		}

		turner.translate(event)
		s.notifyEvent(ctx, sessionID, event)
	}
}

// turnTranslator converts one agent run's flat provider events into the
// desktop wire structure: model-stream parts pass through (turn triplets arrive
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

// stripResultImages returns a copy of r with inline image parts replaced by a
// short text placeholder. Subscribers (the desktop) don't render tool-result
// images, and multi-megabyte base64 frames stall their SSE parser. The model
// still receives the original result.
func stripResultImages(r tools.ToolResult) tools.ToolResult {
	const placeholder = "[image omitted from event stream]"
	switch v := r.Content.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, item := range v {
			out[i] = item
			if item["type"] == "image" {
				out[i] = map[string]any{"type": "text", "text": placeholder}
			}
		}
		r.Content = out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
			if m, ok := item.(map[string]any); ok && m["type"] == "image" {
				out[i] = map[string]any{"type": "text", "text": placeholder}
			}
		}
		r.Content = out
	}
	return r
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
			Part: &consolev1.ModelStreamPart{
				Part: &consolev1.ModelStreamPart_Text{Text: event.Text},
			},
		})
	case loop.EventThinking:
		t.hub.Broadcast(loop.Event{
			Kind: loop.EventModelStreamPart,
			Part: &consolev1.ModelStreamPart{
				Part: &consolev1.ModelStreamPart_Thinking{Thinking: event.Text},
			},
		})
	case loop.EventToolCall:
		if event.Call == nil {
			return
		}
		t.hub.Broadcast(loop.Event{
			Kind: loop.EventModelStreamPart,
			Part: &consolev1.ModelStreamPart{
				Part: &consolev1.ModelStreamPart_ToolCall{ToolCall: &consolev1.ToolCallPreview{
					Id: event.Call.ID, Name: event.Call.Name,
				}},
			},
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
		ui := stripResultImages(*event.Result)
		t.hub.Broadcast(loop.Event{Kind: loop.EventToolExecutionResult, Result: &ui})
		t.results = append(t.results, ui)
	case loop.EventModelStreamEnd, loop.EventTurnEnd:
		t.flushTools()
		t.hub.Broadcast(event)
	case loop.EventModelStreamStart:
		// A new provider turn closes any dangling tool phase first,
		// keeping execution-end strictly before the next stream start.
		t.flushTools()
		t.hub.Broadcast(event)
	case loop.EventUsage, loop.EventTurnDone:
		// Token accounting and the terminal marker stay internal; they are
		// not streamed and the desktop cannot parse them.
	default:
		t.hub.Broadcast(event)
	}
}

// systemSections labels the built prompt's blocks for request breakdowns.
func systemSections(sections []systemprompt.Section) []loop.NamedText {
	out := make([]loop.NamedText, len(sections))
	for i, section := range sections {
		out[i] = loop.NamedText{Name: section.Name, Content: section.Content}
	}
	return out
}
