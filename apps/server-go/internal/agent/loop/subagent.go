// Subagent tool: delegate a focused sub-task to an isolated nested agent
// run. Port of apps/server/agent/src/tools/subagent.ts: same input shape,
// no-subagent recursion, start/activity/end lifecycle events, text summary
// result. The nested run persists nothing (in-memory agent); lifecycle is
// reported through OnEvent for the parent to broadcast/persist.
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/permissions"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

// SubagentContext wires a subagent tool run. A nil context yields the
// simulated no-model response (TS parity for static registration).
type SubagentContext struct {
	Provider     Provider
	Tools        []tools.Tool
	SystemPrompt string
	// Setup is the parent's per-session setup, sent as a leading message.
	Setup    string
	Approver Approver
	// OnEvent receives subagent lifecycle events (start/activity/end).
	OnEvent func(Event)
	// Usage, when set, receives each finished subagent run's token usage.
	Usage *UsageTracker
}

// SubagentStartInfo opens a subagent run.
type SubagentStartInfo struct {
	SubagentID     string `json:"subagentId"`
	ParentToolCall string `json:"parentToolCallId,omitempty"`
	Name           string `json:"name"`
	Role           string `json:"role"`
	Prompt         string `json:"prompt"`
	// MaxTurns mirrors the TS subagent default (10); the desktop requires it.
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
	Prompt string `json:"prompt" jsonschema:"required,description=Clear, actionable task description for the subagent"`
	Name   string `json:"name" jsonschema:"required,description=Name or identifier for the subagent to help differentiate it from other instances"`
	Role   string `json:"role,omitempty" jsonschema:"description=Role or job title for the subagent (e.g. 'Codebase Inspector', 'Test Runner')"`
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
	return t.context.run(ctx, call.ID, in.Prompt, displayName, role)
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

	nested := make([]tools.Tool, 0, len(c.Tools))
	for _, t := range c.Tools {
		if t.Name() == "subagent" {
			continue
		}
		nested = append(nested, t)
	}
	registry := tools.NewRegistry(nested...)
	agent := New(c.Provider, NewExecutor(registry, permissions.FullAccess, c.Approver), nil)
	agent.SystemPrompt = fmt.Sprintf("You are a specialized subagent (%s). Execute the task thoroughly and summarize your findings cleanly.\n%s", role, c.SystemPrompt)
	agent.Setup = c.Setup

	if c.Usage != nil {
		defer func() { c.Usage.AddSubagent(agent.Usage.Snapshot()) }()
	}
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

// textResult wraps a formatted string as the MCP-style content array the TS
// server always sends over the wire (mirrors tools.textResult; duplicated
// here since it is unexported across the package boundary).
func textResult(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}

func randomSubSession() string {
	return "subagent-session-" + newMessageID()[4:]
}
