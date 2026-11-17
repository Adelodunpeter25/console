package codemode

import (
	"context"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
)

type codeInput struct {
	Code      string `json:"code" jsonschema:"required,description=The JavaScript code to execute in the session REPL."`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Evaluation timeout in milliseconds (default 300000, max 600000)."`
}

type codeResetInput struct{}

// NewCodeTool returns the harness code tool.
func NewCodeTool(rt *Runtime) tools.Tool {
	return tools.NewTool(
		"code",
		"Execute JavaScript in the session REPL to orchestrate file operations, shell commands, searches, and tools. Built-ins include $`cmd`, file, write, edit, glob, grep, list, fetch, webSearch, todo, ask, browser, console, mcp, tools, store, spawnContext — call help({query}) to search their docs or describe({name}) for one.",
		tools.TierRead,
		func(ctx context.Context, in codeInput) (any, error) {
			timeout := time.Duration(in.TimeoutMs) * time.Millisecond
			if timeout <= 0 {
				timeout = 5 * time.Minute
			} else if timeout > 10*time.Minute {
				timeout = 10 * time.Minute
			}
			return rt.Eval(ctx, in.Code, timeout)
		},
	)
}

// NewCodeResetTool returns the harness code_reset tool.
func NewCodeResetTool(rt *Runtime) tools.Tool {
	return tools.NewTool(
		"codeReset",
		"Reset the session JavaScript REPL state (clears all variables and in-memory session store).",
		tools.TierRead,
		func(ctx context.Context, in codeResetInput) (any, error) {
			if err := rt.Reset(); err != nil {
				return nil, tools.NewToolError("failed to reset codemode session: %v", err)
			}
			return "Code REPL session state reset.", nil
		},
	)
}

// SetupHint is included in the model's setup instructions when Code Mode is active.
const SetupHint = `# Code Mode Active
You interact with the workspace and tools through the ` + "`code`" + ` tool (JavaScript REPL).
Direct tools are replaced by asynchronous JavaScript functions inside the script.
PLAN MODE: if the user asks for changes, call await requestFullAccess("short reason") inside code FIRST — before any write attempt.

- Builtins ($` + "`cmd`" + `, file, write, edit, glob, grep, list, fetch, webSearch, todo, ask, askMany, browser, console, mcp, tools, store, load, log, preview, visualize, readChat, readSkill, writePlan, requestFullAccess, spawnContext): call help({query: "shell"}) to search or help() to list all; describe({name: "edit"}) for full docs on one.
- Harness tools: listTools(), searchTools(query), describeTool(name), tools.<name>(input).
- MCP servers: mcp.servers(), mcp.tools({id}), mcp.call({id, tool, args}). Do NOT use fetch/webSearch for configured MCP services.
- Subagents: spawnContext({prompt, role}).

Pattern example:
` + "```js" + `
const status = (await $` + "`git status --porcelain`" + `).text();
if (status) log("Working tree dirty:", status);
await edit("config.json", '"debug": false', '"debug": true');
const test = await $` + "`npm test`" + `.nothrow();
if (!test.ok) throw new Error("Tests failed: " + test.stderr);
` + "```" + `
`
