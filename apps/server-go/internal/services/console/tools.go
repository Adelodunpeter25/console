// The two harness tools the console kernel exposes: `console` runs JavaScript
// against the session's persistent runtime, `console_reset` clears it.
//
// These tools replace the ports and project_scripts schemas with two fixed
// two-field schemas. The console API reaches the model once, as documented
// methods in the console-control skill, instead of as definitions on every
// request.
package console

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type consoleInput struct {
	Code      string `json:"code" jsonschema:"description=JavaScript to run with the preinstalled console object, e.g. console.ports.list(). Variables persist between calls; use print() for narration."`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Execution budget in milliseconds. Defaults to 300000 (5 minutes); a runaway script is stopped and reported as unknown completion."`
}

// consoleTool runs code in the session's kernel. It embeds the declarative
// tool for its name, description and schema, and overrides Execute to bind
// the call to one session — the same pattern as the computer tool.
type consoleTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
	projectID string
	cwd       string
	mode      string
}

// NewConsoleTool builds the `console` tool bound to one chat session, so
// its variables never leak into another session's kernel.
func NewConsoleTool(manager *Manager, sessionID, projectID, cwd, mode string) tools.Tool {
	inner := tools.NewTool("console",
		"Run JavaScript against the console API (console.ports.list(), console.approvalModes(), console.sessions.read({session_id}), ...). One call can manage ports, run scripts, and control sessions. Variables persist until console_reset. The console-control instructions in this conversation document the methods.",
		tools.TierExec,
		func(ctx context.Context, in consoleInput) (any, error) {
			return nil, tools.NewToolError("console requires call context")
		})
	return &consoleTool{Tool: inner, manager: manager, sessionID: sessionID, projectID: projectID, cwd: cwd, mode: mode}
}

func (t *consoleTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	var in consoleInput
	if err := json.Unmarshal(arguments, &in); err != nil {
		return nil, tools.NewToolError("Invalid arguments for console: %v", err)
	}
	timeout := time.Duration(in.TimeoutMs) * time.Millisecond
	runtime, err := t.manager.sessionRuntime(t.sessionID, t.projectID, t.cwd)
	if err == nil {
		// The run's mode can change between turns; the ceiling is always the
		// current one.
		runtime.SetCeiling(t.mode)
	}
	if err != nil {
		return nil, tools.NewToolError("console kernel unavailable: %v", err)
	}
	return runtime.Eval(ctx, in.Code, timeout)
}

// consoleResetTool clears the session kernel. It embeds the declarative
// tool the same way consoleTool does.
type consoleResetTool struct {
	tools.Tool
	manager   *Manager
	sessionID string
}

// NewConsoleResetTool builds the `console_reset` tool: clearing variables
// changes nothing on the machine, so it is a read-tier lifecycle operation.
func NewConsoleResetTool(manager *Manager, sessionID string) tools.Tool {
	inner := tools.NewTool("console_reset",
		"Forget the console session: clear every variable and binding created by prior console calls. The console object itself is reinstalled fresh.",
		tools.TierRead,
		func(ctx context.Context, in struct{}) (any, error) {
			return nil, tools.NewToolError("console_reset requires call context")
		})
	return &consoleResetTool{Tool: inner, manager: manager, sessionID: sessionID}
}

func (t *consoleResetTool) Execute(ctx context.Context, arguments json.RawMessage) (any, error) {
	t.manager.dropRuntime(t.sessionID)
	return textResult("Console session cleared. Variables are gone; call console to start fresh."), nil
}

func textResult(text string) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}}
}
