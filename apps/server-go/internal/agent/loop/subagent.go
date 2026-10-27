// Subagent tool: delegate a focused sub-task to an isolated nested agent
// run: no-subagent recursion, start/activity/end lifecycle events, text
// summary result. The nested run persists nothing (in-memory agent);
// lifecycle is reported through OnEvent for the parent to broadcast/persist.
package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

// SubagentContext wires a subagent tool run. A nil context yields the
// simulated no-model response (used for static registration).
type SubagentContext struct {
	Provider Provider
	// Model is the parent's model ID; the nested run reuses it (providers
	// such as Claude reject requests with an empty model).
	Model string
	Tools []tools.Tool
	// ToolSource, when set, overrides Tools with the parent's tools as they
	// are when the subagent starts, so groups the parent loaded mid-run
	// (e.g. MCP servers) carry over.
	ToolSource func() []tools.Tool
	// NestedTools, when set, receives the subagent's own registry and may
	// add tools bound to it (e.g. a loadTools that loads into the subagent).
	NestedTools  func(registry *tools.Registry)
	SystemPrompt string
	// Setup is the parent's per-session setup, sent as a leading message.
	Setup string
	// ApprovalMode is the parent run's permission mode. A subagent inherits its
	// parent's tools, so it must inherit the parent's authority too: it may not
	// act with more permission than the run that spawned it. Zero value means
	// always-ask, which is the safe default for a run that did not say.
	ApprovalMode permissions.Mode
	// Guard is the parent run's path guard. A subagent holds the same tools,
	// so it must be confined to the same project root; nil means unguarded.
	Guard    *permissions.Guard
	Approver Approver
	// OnEvent receives subagent lifecycle events (start/activity/end).
	OnEvent func(Event)
	// Usage, when set, receives each finished subagent run's token usage.
	Usage *UsageTracker
	// BackgroundWork, when set, registers detached subagent runs and queues
	// their terminal summaries without blocking the parent tool call.
	BackgroundWork *services.BackgroundWorkManager
	SessionID      string
	ParentRunID    string
	// AccountID pins the subagent to the parent run's account credential
	// (decision 7): the nested agent's turns load that account's file
	// directly instead of re-resolving the provider's active account.
	AccountID string
}

// SubagentStartInfo opens a subagent run.
type SubagentStartInfo struct {
	SubagentID     string `json:"subagentId"`
	ParentToolCall string `json:"parentToolCallId,omitempty"`
	Name           string `json:"name"`
	Role           string `json:"role"`
	Prompt         string `json:"prompt"`
	// MaxTurns is the nested run's turn limit (default 10); the desktop requires it.
	MaxTurns int `json:"maxTurns"`
}

// SubagentActivityInfo tracks one nested tool call.
type SubagentActivityInfo struct {
	SubagentID string `json:"subagentId"`
	TurnIndex  int    `json:"turnIndex"`
	ToolCallID string `json:"toolCallId,omitempty"`
	ToolName   string `json:"toolName,omitempty"`
	Args       any    `json:"args,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// SubagentEndInfo closes a subagent run.
type SubagentEndInfo struct {
	SubagentID string `json:"subagentId"`
	Status     string `json:"status"` // completed | aborted | error
	Summary    string `json:"summary,omitempty"`
	Error      string `json:"error,omitempty"`
	TotalTurns int    `json:"totalTurns"`
}

type subagentInput struct {
	Prompt     string `json:"prompt" jsonschema:"required,description=Clear, actionable task description for the subagent"`
	Name       string `json:"name" jsonschema:"required,description=Name or identifier for the subagent to help differentiate it from other instances"`
	Role       string `json:"role,omitempty" jsonschema:"description=Role or job title for the subagent (e.g. 'Codebase Inspector', 'Test Runner')"`
	Background bool   `json:"background,omitempty" jsonschema:"description=Start the subagent in the background and continue immediately. The result is delivered through the background-work completion callback and the next parent model turn."`
}

// NewSubagentTool builds the "subagent" tool. With a nil context it
// returns the simulated response (no nested run).
func NewSubagentTool(ctx *SubagentContext) tools.Tool {
	inner := tools.NewTool("subagent", "Delegate a focused sub-task to an isolated subagent.", tools.TierRead,
		func(ctx context.Context, in subagentInput) (any, error) {
			return nil, tools.NewToolError("subagent requires call context")
		})
	return &subagentTool{Tool: inner, context: ctx}
}

type subagentTool struct {
	tools.Tool
	context *SubagentContext
}

func (t *subagentTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	return t.ExecuteCall(ctx, tools.ToolCall{Arguments: arguments})
}

func (t *subagentTool) ExecuteCall(ctx context.Context, call tools.ToolCall) (any, error) {
	var in subagentInput
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &in); err != nil {
			return nil, tools.NewToolError("Invalid arguments for subagent: %v", err)
		}
	}
	role := in.Role
	if role == "" {
		role = "Subagent Researcher"
	}
	displayName := in.Name
	if displayName == "" {
		displayName = role
	}
	if t.context == nil {
		return textResult(fmt.Sprintf("Subagent [%s] simulated run for: %q\n(No active provider attached to task tool context)", displayName, in.Prompt)), nil
	}
	if in.Background {
		return t.context.startBackground(ctx, call.ID, in.Prompt, displayName, role)
	}
	return t.context.run(ctx, call.ID, in.Prompt, displayName, role)
}

func (c *SubagentContext) startBackground(ctx context.Context, parentCallID, prompt, name, role string) (any, error) {
	if c.BackgroundWork == nil || c.SessionID == "" {
		return nil, tools.NewToolError("Background subagents are not available in this context.")
	}
	work, err := c.BackgroundWork.Register(services.BackgroundWorkOptions{
		SessionID: c.SessionID, ParentRunID: c.ParentRunID, ParentCallID: parentCallID,
		Kind:     services.BackgroundWorkSubagent,
		Metadata: map[string]string{"name": name, "role": role},
	})
	if err != nil {
		return nil, tools.NewToolError("Could not start background subagent: %v", err)
	}
	childCtx, cancel := context.WithCancel(context.Background())
	if err := c.BackgroundWork.SetCancel(work.ID, cancel); err != nil {
		cancel()
		return nil, tools.NewToolError("Could not start background subagent: %v", err)
	}
	go func() {
		result, runErr := c.run(childCtx, parentCallID, prompt, name, role)
		status := services.BackgroundWorkCompleted
		summary := subagentSummary(result)
		failure := ""
		if runErr != nil {
			if errors.Is(childCtx.Err(), context.Canceled) {
				status = services.BackgroundWorkCancelled
				failure = "Subagent was cancelled"
			} else {
				status = services.BackgroundWorkFailed
				failure = runErr.Error()
			}
			summary = ""
		}
		// The nested runner emits detailed lifecycle events. The manager owns
		// the durable, compact completion callback.
		_, _ = c.BackgroundWork.Complete(work.ID, c.SessionID, status, summary, failure,
			map[string]string{"subagent_id": work.ID})
	}()
	return textResult(fmt.Sprintf("Started background subagent [%s].\nBackground work: %s\nThe result will be delivered when the parent is ready for a later turn.", name, work.ID)), nil
}

func (c *SubagentContext) run(ctx context.Context, parentCallID, prompt, name, role string) (any, error) {
	subagentID := "subagent-" + newMessageID()[4:]
	emit := func(kind EventKind, subagent any) {
		if c.OnEvent != nil {
			c.OnEvent(Event{Kind: kind, Subagent: subagent})
		}
	}
	emit(EventSubagentStart, SubagentStartInfo{
		SubagentID: subagentID, ParentToolCall: parentCallID,
		Name: name, Role: role, Prompt: prompt, MaxTurns: 10,
	})

	source := c.Tools
	if c.ToolSource != nil {
		source = c.ToolSource()
	}
	nested := make([]tools.Tool, 0, len(source))
	for _, t := range source {
		// No recursion; loadTools is re-bound to the nested registry below.
		if t.Name() == "subagent" || (c.NestedTools != nil && t.Name() == "loadTools") {
			continue
		}
		nested = append(nested, t)
	}
	registry := tools.NewRegistry(nested...)
	if c.NestedTools != nil {
		c.NestedTools(registry)
	}
	mode := c.ApprovalMode
	if mode == "" {
		mode = permissions.AlwaysAsk
	}
	executor := NewExecutor(registry, mode, c.Approver)
	executor.Guard = c.Guard
	agent := New(c.Provider, executor, nil)
	agent.SystemPrompt = fmt.Sprintf("You are a specialized subagent (%s). Execute the task thoroughly and summarize your findings cleanly.\n%s", role, c.SystemPrompt)
	agent.Setup = c.Setup
	agent.Model = c.Model
	agent.AccountID = c.AccountID

	if c.Usage != nil {
		defer func() { c.Usage.AddSubagent(agent.Usage.Snapshot()) }()
	}
	agent.ToolDefs = registry.Definitions
	events, err := agent.Run(ctx, randomSubSession(), prompt, registry.Definitions())
	if err != nil {
		emit(EventSubagentEnd, SubagentEndInfo{SubagentID: subagentID, Status: "error", Error: err.Error()})
		return nil, err
	}
	var summary strings.Builder
	turnIndex, totalTurns := 0, 0
	callArgs := map[string]any{}
	for {
		event, err, ok := events.Next()
		if !ok {
			if err != nil {
				if ctx.Err() != nil {
					emit(EventSubagentEnd, SubagentEndInfo{SubagentID: subagentID, Status: "aborted", Summary: fmt.Sprintf("Subagent [%s] cancelled by user abort.", name), TotalTurns: totalTurns})
					return fmt.Sprintf("Subagent [%s] cancelled by user abort.", name), tools.NewToolError("Subagent [%s] cancelled by user abort.", name)
				}
				emit(EventSubagentEnd, SubagentEndInfo{SubagentID: subagentID, Status: "error", Error: err.Error(), TotalTurns: totalTurns})
				return nil, err
			}
			break
		}
		switch event.Kind {
		case EventText:
			summary.WriteString(event.Text)
		case EventToolCall:
			if event.Call != nil {
				totalTurns = turnIndex + 1
				callArgs[event.Call.ID] = jsonAny(event.Call.Arguments)
				emit(EventSubagentActivity, SubagentActivityInfo{
					SubagentID: subagentID, TurnIndex: turnIndex,
					ToolCallID: event.Call.ID, ToolName: event.Call.Name,
					Args: callArgs[event.Call.ID], Status: "running",
				})
			}
		case EventToolResult:
			if event.Result != nil {
				status := "completed"
				errText := ""
				if event.Result.IsError {
					status = "error"
					errText = fmt.Sprint(event.Result.Content)
				}
				emit(EventSubagentActivity, SubagentActivityInfo{
					SubagentID: subagentID, TurnIndex: turnIndex,
					ToolCallID: event.Result.ToolCallID, ToolName: event.Result.ToolName,
					Args: callArgs[event.Result.ToolCallID], Status: status, Error: errText,
				})
				turnIndex++
			}
		}
	}
	final := strings.TrimSpace(summary.String())
	if final == "" {
		final = "Subagent finished with no text output."
	}
	emit(EventSubagentEnd, SubagentEndInfo{SubagentID: subagentID, Status: "completed", Summary: final, TotalTurns: totalTurns})
	return textResult(fmt.Sprintf("Subagent [%s] Completed Task:\n%s", name, final)), nil
}

// textResult wraps a formatted string as the MCP-style content array that is
// always sent over the wire (mirrors tools.textResult; duplicated here since
// it is unexported across the package boundary).
func textResult(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

func subagentSummary(result any) string {
	parts, ok := result.([]map[string]any)
	if !ok {
		return "Subagent completed."
	}
	for _, part := range parts {
		if text, ok := part["text"].(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return "Subagent completed."
}

func randomSubSession() string {
	return "subagent-session-" + newMessageID()[4:]
}
