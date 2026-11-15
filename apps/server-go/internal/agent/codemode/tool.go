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
		"Execute JavaScript in the session REPL to orchestrate file operations, shell commands, searches, and tools. Built-ins: $`cmd`, file(path), write(path, content), edit(path, old, new), glob(pat), grep(pat, opts), list(path), fetch(url), webSearch(query), todo(op, arg), visualize(spec), tools.X(args), searchTools(query), describeTool(name), log(...args), spawnContext(args).",
		tools.TierExec,
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
		"code_reset",
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
Direct tools are replaced by asynchronous JavaScript functions inside the script:
- $` + "`cmd`" + ` runs shell commands (e.g. ` + "`await $'git status'`" + `). Returns object with .text(), .json(), and .nothrow()
- file(path) reads file text contents
- write(path, content) writes/overwrites file
- edit(path, oldStr, newStr) performs exact replacement
- glob(pattern, [root]) finds matching file paths
- grep(pattern, [options]) searches file contents
- list(path) lists directory entries
- fetch(url, [options]) fetches web page markdown or text
- webSearch(query, [options]) searches the web (DuckDuckGo/Brave)
- todo(op, [arg]) manages the session task list (e.g. todo('view'), todo('start', 1), todo('done', 1))
- visualize(spec) renders charts/cards in chat
- readChat(ref, [limit]) reads previous chat transcript
- tools.<name>(input) invokes any registered tool (built-in or MCP)
- searchTools(query) and describeTool(name) find and inspect available tool schemas
- log(...args) outputs narration / intermediate text
- spawnContext({ prompt, role }) runs a subagent
`
