package codemode

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// builtinDoc is one JavaScript builtin's model-facing documentation. The
// catalog is the single source of truth for help() and describe() in the
// REPL, so builtin docs cannot drift from what the bootstrap installs.
type builtinDoc struct {
	name      string
	group     string
	signature string
	doc       string
}

// builtinCatalog documents every global the bootstrap script installs, plus
// the bridge namespaces (mcp, tools, console, browser). Group is the
// namespace help({group}) filters on.
var builtinCatalog = []builtinDoc{
	// Shell
	{name: "$", group: "shell", signature: "$`cmd` | $(\"cmd\") | $({cwd:\"dir\"})`cmd`", doc: "Run a shell command. Returns {code, exitCode, stdout, stderr, ok, durationMs, text(), json()}. Throws on non-zero exit unless .nothrow() is called: (await $`git status`).nothrow()"},

	// Files
	{name: "file", group: "files", signature: "file(path)", doc: "Read a file's text contents. Relative to the process CWD or absolute. Aliases: readFile."},
	{name: "write", group: "files", signature: "write(path, content)", doc: "Write/overwrite a file, creating parent directories. Aliases: writeFile."},
	{name: "edit", group: "files", signature: "edit(path, oldStr, newStr)", doc: "Exact string replacement in a file; throws if oldStr is missing or not unique. Aliases: editFile."},
	{name: "glob", group: "files", signature: "glob(pattern, [root])", doc: "Find file paths matching a glob pattern."},
	{name: "grep", group: "files", signature: "grep(pattern, [options])", doc: "Search file contents for a pattern."},
	{name: "list", group: "files", signature: "list(path)", doc: "List directory entries. Aliases: listDir."},

	// Web
	{name: "fetch", group: "web", signature: "fetch(url, [options])", doc: "Fetch a web page as markdown or text. Aliases: webFetch."},
	{name: "webSearch", group: "web", signature: "webSearch(query, [options])", doc: "Search the web (DuckDuckGo/Brave)."},

	// User interaction
	{name: "ask", group: "user", signature: "ask(question, [options], [isMultiSelect])", doc: "Ask the user a clarifying question; returns their answer."},
	{name: "askMany", group: "user", signature: "askMany(questions)", doc: "Ask the user multiple questions in sequence."},

	// Session task list
	{name: "todo", group: "session", signature: "todo(op, [arg])", doc: "Manage the session task list, e.g. todo('view'), todo('start', 1), todo('done', 1)."},

	// Artifacts and context
	{name: "visualize", group: "artifacts", signature: "visualize(spec)", doc: "Render a chart, table, or metric card in the chat."},
	{name: "preview", group: "artifacts", signature: "preview(data)", doc: "Preview arbitrary data in the chat."},
	{name: "readChat", group: "artifacts", signature: "readChat(ref, [limit])", doc: "Read a previous chat transcript."},
	{name: "readSkill", group: "skills", signature: "readSkill([name])", doc: "Read a built-in skill's full instructions before first use."},
	{name: "writePlan", group: "plan", signature: "writePlan(name, content)", doc: "Save a Markdown design plan to docs/plan/<name>.md (plan mode)."},
	{name: "requestFullAccess", group: "plan", signature: "requestFullAccess(reason)", doc: "Request user approval to escalate from plan mode to Full Access. Call before any write attempt in plan mode."},

	// Browser
	{name: "browser", group: "browser", signature: "browser.open({url}) | browser.snapshot() | browser(code)", doc: "Drive the built-in browser: navigate, observe, extract, and clean up. Also browser.tabs(), browser.close({tabId})."},

	// Console desktop control plane
	{name: "console", group: "console", signature: "console.<method>(args)", doc: "Operate the app: ports, scripts, sessions, projects, MCP servers. Discover methods with console.help({query}) and console.describe({name}). console.log/print still work for narration."},

	// MCP servers
	{name: "mcp.servers", group: "mcp", signature: "mcp.servers()", doc: "List configured MCP servers."},
	{name: "mcp.tools", group: "mcp", signature: "mcp.tools({id})", doc: "Discover an MCP server's tools."},
	{name: "mcp.call", group: "mcp", signature: "mcp.call({id, tool, args})", doc: "Call an MCP tool directly. Do NOT use fetch/webSearch for configured MCP services."},

	// Harness tools
	{name: "tools", group: "tools", signature: "tools.<name>(input)", doc: "Invoke any registered harness tool directly; results with JSON text are auto-parsed."},
	{name: "listTools", group: "tools", signature: "listTools()", doc: "List all registered harness and MCP tools with summaries."},
	{name: "searchTools", group: "tools", signature: "searchTools(query)", doc: "Find tools by keyword."},
	{name: "describeTool", group: "tools", signature: "describeTool(name)", doc: "Inspect one tool's full schema."},
	{name: "tools.loadTools", group: "tools", signature: "tools.loadTools({group:\"mcp:<id>\"})", doc: "Legacy way to add MCP tools to tools.X; mcp.* is preferred."},

	// Session state and orchestration
	{name: "store", group: "session", signature: "store(key, value)", doc: "Persist a value in the session store across evaluations."},
	{name: "load", group: "session", signature: "load(key)", doc: "Read a value from the session store."},
	{name: "log", group: "session", signature: "log(...args)", doc: "Output narration / intermediate text."},
	{name: "spawnContext", group: "subagents", signature: "spawnContext({prompt, role})", doc: "Run a subagent."},
	{name: "help", group: "core", signature: "help([query]) | help({query, group})", doc: "Search builtin docs by keyword, or list a group's builtins. help() lists everything."},
	{name: "describe", group: "core", signature: "describe(name | {name})", doc: "Full documentation for one builtin."},
}

// helpArgs reads the optional query/group arguments help() accepts.
func helpArgs(args map[string]any) (query, group string) {
	if s, ok := args["query"].(string); ok {
		query = s
	}
	if s, ok := args["group"].(string); ok {
		group = s
	}
	return query, group
}

// scoreBuiltin rates a doc against a whitespace-separated query; every word
// must appear in the name, signature or doc. Name hits score highest.
func scoreBuiltin(d builtinDoc, query string) int {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return 1
	}
	name := strings.ToLower(d.name)
	hay := strings.ToLower(d.name + " " + d.signature + " " + d.doc + " " + d.group)
	score := 0
	for _, w := range words {
		switch {
		case strings.Contains(name, w):
			score += 3
		case strings.Contains(hay, w):
			score++
		default:
			return 0
		}
	}
	return score
}

func builtinEntry(d builtinDoc) map[string]any {
	return map[string]any{
		"name":      d.name,
		"group":     d.group,
		"signature": d.signature,
		"doc":       d.doc,
	}
}

// helpHandler implements help({query?, group?}) in the REPL.
func helpHandler(args map[string]any) (*jskernel.Result, error) {
	query, group := helpArgs(args)
	var matches []builtinDoc
	for _, d := range builtinCatalog {
		if group != "" && d.group != group {
			continue
		}
		if scoreBuiltin(d, query) > 0 {
			matches = append(matches, d)
		}
	}
	if len(matches) == 0 {
		return marshalDocs(map[string]any{
			"matches": []any{},
			"note":    "no builtins matched; call help() to list everything, or searchTools()/describeTool() for harness tools",
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return scoreBuiltin(matches[i], query) > scoreBuiltin(matches[j], query)
	})
	if query != "" && len(matches) > 15 {
		matches = matches[:15]
	}
	out := make([]map[string]any, 0, len(matches))
	for _, d := range matches {
		out = append(out, builtinEntry(d))
	}
	return marshalDocs(map[string]any{"matches": out})
}

// describeHandler implements describe(name) in the REPL.
func describeHandler(args map[string]any) (*jskernel.Result, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("describe needs a name, e.g. describe(\"edit\"); call help() to list builtins")
	}
	name = strings.TrimPrefix(strings.TrimSpace(name), "console.")
	for _, d := range builtinCatalog {
		if d.name == name {
			return marshalDocs(builtinEntry(d))
		}
	}
	return nil, fmt.Errorf("unknown builtin %q; call help({query: %q}) to find it", name, name)
}

func marshalDocs(v any) (*jskernel.Result, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return &jskernel.Result{StructuredContent: data}, nil
}
