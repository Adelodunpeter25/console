// The two harness tools computer use exposes: `computer` runs JavaScript
// against the session's persistent runtime, `computer_reset` clears it.
//
// Two tools with small fixed schemas replace 56 per-tool schemas (~28K tokens
// re-sent every turn). The driver API reaches the model once, as documented
// methods in the /computer-use skill, instead of as definitions on every
// request.
package cua

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type computerInput struct {
	Code      string `json:"code" jsonschema:"description=JavaScript to run with the preinstalled cua object, e.g. cua.get_window_state({pid, window_id}). Variables persist between calls; use print() for narration."`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Execution budget in milliseconds. Defaults to 300000 (5 minutes); a runaway script is stopped and reported as unknown completion."`
}

// computerTool runs code in the session's kernel. It embeds the declarative
// tool for its name, description and schema, and overrides Execute to bind
// the call to one session — the same pattern as the subagent tool.
type computerTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
}

// NewComputerTool builds the `computer` tool bound to one chat session, so
// its variables never leak into another session's kernel.
func NewComputerTool(manager *Manager, sessionID string) tools.Tool {
	inner := tools.NewTool("computer",
		"Run JavaScript against the computer with the preinstalled cua object (cua.list_apps(), cua.click({...}), ...). One call can observe, pick and act; variables persist until computer_reset. The /computer-use instructions in this conversation document the methods.",
		tools.TierExec,
		func(ctx context.Context, in computerInput) (any, error) {
			return nil, tools.NewToolError("computer requires call context")
		})
	return &computerTool{Tool: inner, manager: manager, sessionID: sessionID}
}

func (t *computerTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var in computerInput
	if err := json.Unmarshal(arguments, &in); err != nil {
		return nil, tools.NewToolError("Invalid arguments for computer: %v", err)
	}
	timeout := time.Duration(in.TimeoutMs) * time.Millisecond
	runtime, err := t.manager.sessionRuntime(t.sessionID)
	if err != nil {
		return nil, &ErrNoDriver{Cause: err}
	}
	return runtime.Eval(ctx, in.Code, timeout)
}

// computerResetTool clears the session kernel. It embeds the declarative
// tool the same way computerTool does.
type computerResetTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
}

// NewComputerResetTool builds the `computer_reset` tool: clearing variables
// changes nothing on the machine, so it is a read-tier lifecycle operation
// like loadTools.
func NewComputerResetTool(manager *Manager, sessionID string) tools.Tool {
	inner := tools.NewTool("computer_reset",
		"Forget the computer-use session: clear every variable and binding created by prior computer calls. The cua object itself is reinstalled fresh.",
		tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) {
			return nil, tools.NewToolError("computer_reset requires call context")
		})
	return &computerResetTool{Tool: inner, manager: manager, sessionID: sessionID}
}

func (t *computerResetTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	t.manager.dropRuntime(t.sessionID)
	return textResult("Computer-use session cleared. Variables are gone; call computer to start fresh."), nil
}

func textResult(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}
