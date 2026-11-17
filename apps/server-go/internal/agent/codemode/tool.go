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
		"Execute JavaScript in the session REPL to orchestrate file operations, shell commands, searches, and tools. Built-ins: $`cmd`, file(path), write(path, content), edit(path, old, new), glob(pat), grep(pat, opts), list(path), fetch(url), webSearch(query), mcp.servers(), mcp.tools({id}), mcp.call({id,tool,args}), todo(op, arg), ask(q, opts), writePlan(name, content), requestFullAccess(reason), visualize(spec), tools.X(args), searchTools(query), describeTool(name), log(...args), spawnContext(args).",
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
- $` + "`cmd`" + ` or $("cmd") runs shell commands. Custom options: $({ cwd: "dir" })` + "`cmd`" + `.
  Returns { code, exitCode, stdout, stderr, ok, durationMs, text(), json() }.
  Throws on non-zero exit code unless .nothrow() is called: (await $` + "`git status`" + `).nothrow()
- file(path) reads file text contents. Relative to process CWD or absolute.
- write(path, content) writes/overwrites file (creates parent directories automatically).
- writePlan(name, content) saves Markdown design plan to docs/plan/<name>.md (in plan mode).
- requestFullAccess(reason) requests user approval to escalate from plan mode to Full Access.
- edit(path, oldStr, newStr) performs exact replacement (throws if oldStr is missing or not unique).
- glob(pattern, [root]) finds matching file paths.
- grep(pattern, [options]) searches file contents.
- list(path) lists directory entries.
- fetch(url, [options]) fetches web page markdown or text.
- webSearch(query, [options]) searches the web (DuckDuckGo/Brave).
- External systems (Jira etc.): call await mcp.servers() to see configured MCP servers, await mcp.tools({id:"jira"}) to discover their tools, then await mcp.call({id:"jira", tool:"searchIssues", args:{...}}). Do NOT use fetch/webSearch for configured MCP services.
- tools.loadTools({group:"mcp:<id>"}) remains available as a legacy way to add MCP tools to tools.X; mcp.* is preferred.
- todo(op, [arg]) manages the session task list (e.g. todo('view'), todo('start', 1), todo('done', 1)).
- ask(question, [options], [isMultiSelect]) asks the user a clarifying question (returns user answer).
- askMany(questions) asks the user multiple questions in sequence.
- readSkill([name]) reads full instructions for a built-in skill (e.g. readSkill("browser-use")).
- browser: browser.open({url}), browser.snapshot(), browser.tabs(), browser.close({tabId}), or browser(code).
- console: console.ports.*, console.sessions.*, console.scripts.*, console.projects.*, console.mcp.*, console.models() (while console.log remains). Discover methods with console.help({query}) and console.describe({name}).
- listTools() lists all registered harness and MCP tools with summaries.
- searchTools(query) and describeTool(name) find and inspect available tool schemas.
- tools.<name>(input) invokes any registered tool directly (results with JSON text are auto-parsed).
- visualize(spec) renders charts/cards in chat.
- readChat(ref, [limit]) reads previous chat transcript.
- log(...args) outputs narration / intermediate text.
- spawnContext({ prompt, role }) runs a subagent.

Pattern example:
` + "```js" + `
const status = (await $` + "`git status --porcelain`" + `).text();
if (status) log("Working tree dirty:", status);
await edit("config.json", '"debug": false', '"debug": true');
const test = await $` + "`npm test`" + `.nothrow();
if (!test.ok) throw new Error("Tests failed: " + test.stderr);
` + "```" + `
`
