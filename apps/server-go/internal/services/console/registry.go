package console

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// str is shorthand for a required-or-optional string argument spec.
func str(required bool, doc string) ArgSpec {
	return ArgSpec{Type: "string", Required: required, Doc: doc}
}

func integer(required bool, doc string) ArgSpec {
	return ArgSpec{Type: "integer", Required: required, Doc: doc}
}

func boolean(required bool, doc string) ArgSpec {
	return ArgSpec{Type: "boolean", Required: required, Doc: doc}
}

func object(required bool, doc string) ArgSpec {
	return ArgSpec{Type: "object", Required: required, Doc: doc}
}

func array(required bool, doc string) ArgSpec {
	return ArgSpec{Type: "array", Required: required, Doc: doc}
}

// consoleCatalog is every method the console object exposes, grouped by
// domain. Handlers are method expressions on *Runtime, so the catalog stays
// data: adding a capability means adding one entry here plus its handler.
func consoleCatalog() []methodDef {
	return []methodDef{
		// Ports
		{name: "ports.list", description: "List the ports forwarded for this project.", handler: (*Runtime).portsList},
		{name: "ports.forward", description: "Forward a local port so it is reachable.", args: map[string]ArgSpec{"port": integer(true, "local port number, 1-65535")}, handler: (*Runtime).portsForward},
		{name: "ports.unforward", description: "Remove a port forward.", args: map[string]ArgSpec{"port": integer(true, "local port number")}, handler: (*Runtime).portsUnforward},

		// Scripts
		{name: "scripts.list", description: "List the project's runnable scripts.", handler: (*Runtime).scriptsList},
		{name: "scripts.run", description: "Run a project script by id.", args: map[string]ArgSpec{
			"id":      str(true, "script id from scripts.list"),
			"wait_ms": integer(false, "milliseconds to wait for the run, capped at 10000"),
		}, handler: (*Runtime).scriptsRun},
		{name: "scripts.status", description: "Inspect one script run, or list recent runs when run_id is omitted.", args: map[string]ArgSpec{"run_id": str(false, "run id from scripts.run")}, handler: (*Runtime).scriptsStatus},
		{name: "scripts.output", description: "Read a run's output in pages; pass next back as cursor for only new output.", args: map[string]ArgSpec{
			"run_id": str(true, "run id"),
			"stream": str(false, "stdout (default) or stderr"),
			"cursor": integer(false, "byte offset to resume from"),
			"limit":  integer(false, "max bytes to return"),
		}, handler: (*Runtime).scriptsOutput},
		{name: "scripts.stop", description: "Stop a running script.", args: map[string]ArgSpec{"run_id": str(true, "run id")}, handler: (*Runtime).scriptsStop},

		// Discovery
		{name: "models", description: "List available providers and models, including thinking levels.", handler: (*Runtime).models},
		{name: "approvalModes", description: "List valid approval modes, their descriptions and the restriction order.", handler: (*Runtime).approvalModes},
		{name: "help", description: "Search console methods by keyword, or list a namespace's methods.", args: map[string]ArgSpec{
			"query":     str(false, "keywords to search names and descriptions"),
			"namespace": str(false, "limit results to one namespace, e.g. sessions"),
		}, handler: helpHandler},
		{name: "describe", description: "Full documentation for one console method: arguments, types and an example call.", args: map[string]ArgSpec{
			"name": str(true, "method name, e.g. sessions.create or console.sessions.create"),
		}, handler: describeHandler},

		// Projects
		{name: "projects.list", description: "List registered projects.", handler: (*Runtime).projectsList},
		{name: "projects.add", description: "Register a project directory.", args: map[string]ArgSpec{
			"directory": str(true, "absolute or relative path to the project directory"),
			"name":      str(false, "display name; defaults to the directory base name"),
		}, handler: (*Runtime).projectsAdd},

		// Sessions: persistence and history
		{name: "sessions.list", description: "List persisted, non-deleted sessions, optionally filtered by project or directory.", args: map[string]ArgSpec{
			"project_id": str(false, "only sessions of this project"),
			"cwd":        str(false, "only sessions with this working directory"),
		}, handler: (*Runtime).sessionsList},
		{name: "sessions.read", description: "Read a session's header and transcript; use limit/before to page.", args: map[string]ArgSpec{
			"session_id":      str(true, "session id"),
			"limit":           integer(false, "only the most recent N messages"),
			"before":          integer(false, "cursor from a previous read's nextCursor"),
			"include_deleted": boolean(false, "read a soft-deleted session"),
		}, handler: (*Runtime).sessionsRead},
		{name: "sessions.delete", description: "Soft-delete a session; it must not be running.", args: map[string]ArgSpec{"session_id": str(true, "session id")}, handler: (*Runtime).sessionsDelete},
		{name: "sessions.search", description: "Find messages containing every word of a query across sessions.", args: map[string]ArgSpec{
			"query":           str(true, "words to search for, case-insensitive"),
			"session_id":      str(false, "search one session"),
			"project_id":      str(false, "search one project's sessions"),
			"cwd":             str(false, "search sessions with this working directory"),
			"all_projects":    boolean(false, "search every session"),
			"roles":           array(false, "message roles to search, e.g. [\"user\",\"assistant\"]"),
			"limit":           integer(false, "max hits overall"),
			"per_session":     integer(false, "max hits per session"),
			"include_deleted": boolean(false, "also search soft-deleted sessions"),
		}, handler: (*Runtime).sessionsSearch},
		{name: "sessions.fork", description: "Copy a session's history through a message into a new idle session.", args: map[string]ArgSpec{
			"session_id": str(true, "source session id"),
			"message_id": str(true, "message id from sessions.read; history is copied through it"),
		}, handler: (*Runtime).sessionsFork},

		// Sessions: execution
		{name: "sessions.create", description: "Create a persistent session; with prompt it also starts a run.", args: map[string]ArgSpec{
			"prompt":         str(false, "first prompt; starts the session working when given"),
			"project_id":     str(false, "project to run in; its path becomes cwd"),
			"cwd":            str(false, "working directory"),
			"model":          str(false, "model id; defaults to this session's model"),
			"provider":       str(false, "provider name; defaults to this session's provider"),
			"thinking_level": str(false, "thinking level, checked against the model"),
			"approval_mode":  str(false, "plan-mode or full-access; clamped to this run's ceiling"),
			"worktree":       object(false, "optional {branch, base_branch} to create a git worktree"),
		}, handler: (*Runtime).sessionsCreate},
		{name: "sessions.prompt", description: "Send a follow-up prompt to a session; it runs now or queues behind the current run.", args: map[string]ArgSpec{
			"session_id":     str(true, "target session id"),
			"prompt":         str(true, "the prompt text"),
			"model":          str(false, "per-turn model override"),
			"provider":       str(false, "per-turn provider override"),
			"thinking_level": str(false, "per-turn thinking level"),
			"approval_mode":  str(false, "per-turn mode, capped at this run's ceiling"),
		}, handler: (*Runtime).sessionsPrompt},
		{name: "sessions.wait", description: "Block until sessions finish their current run, or return a background ticket with background:true.", args: map[string]ArgSpec{
			"session_id":  str(false, "one session to wait on"),
			"session_ids": array(false, "up to 20 session ids to wait on"),
			"mode":        str(false, "all (default) or any"),
			"timeout_ms":  integer(false, "wait budget; default 30000, capped at 60000 (foreground)"),
			"background":  boolean(false, "return immediately and be notified when done"),
		}, handler: (*Runtime).sessionsWait},
		{name: "sessions.abort", description: "Stop a session's active run and clear its queued prompts.", args: map[string]ArgSpec{"session_id": str(true, "session id")}, handler: (*Runtime).sessionsAbort},
		{name: "sessions.queue", description: "Show the prompt waiting to run on a session, if any.", args: map[string]ArgSpec{"session_id": str(true, "session id")}, handler: (*Runtime).sessionsQueue},
		{name: "sessions.clear_queue", description: "Discard every prompt waiting on a session.", args: map[string]ArgSpec{"session_id": str(true, "session id")}, handler: (*Runtime).sessionsClearQueue},

		// Sessions: configuration
		{name: "sessions.set_model", description: "Change a session's model for future runs.", args: map[string]ArgSpec{
			"session_id": str(true, "session id"),
			"model":      str(true, "model id; check console.models()"),
			"provider":   str(false, "provider; defaults to the session's current one"),
		}, handler: (*Runtime).sessionsSetModel},
		{name: "sessions.set_approval", description: "Change a session's approval mode; never above this run's ceiling.", args: map[string]ArgSpec{
			"session_id": str(true, "session id"),
			"mode":       str(true, "plan-mode or full-access"),
		}, handler: (*Runtime).sessionsSetApproval},

		// Automations
		{name: "automation.create", description: "Schedule future work on this session (once or interval); runs in this same thread with full context.", args: map[string]ArgSpec{
			"prompt":           str(true, "prompt to run when the schedule fires"),
			"kind":             str(true, "once or interval"),
			"interval_seconds": integer(false, "repeat interval in seconds (required when kind is interval)"),
			"run_at":           integer(false, "unix timestamp in seconds to fire (required when kind is once)"),
		}, handler: (*Runtime).automationCreate},
		{name: "automation.get", description: "Read one automation by id, or list this session's automations when id is omitted.", args: map[string]ArgSpec{
			"id": str(false, "automation id"),
		}, handler: (*Runtime).automationGet},
		{name: "automation.update", description: "Update an active or paused automation and re-arm its next run from now.", args: map[string]ArgSpec{
			"id":               str(true, "automation id"),
			"prompt":           str(false, "new prompt"),
			"kind":             str(false, "once or interval"),
			"interval_seconds": integer(false, "new interval in seconds"),
			"run_at":           integer(false, "new unix timestamp in seconds"),
		}, handler: (*Runtime).automationUpdate},
		{name: "automation.delete", description: "Delete an automation (call when a monitoring task is complete).", args: map[string]ArgSpec{
			"id": str(true, "automation id"),
		}, handler: (*Runtime).automationDelete},

		// MCP servers
		{name: "mcp.list", description: "List configured MCP servers with their live connection status.", handler: (*Runtime).mcpListImpl},
		{name: "mcp.tools", description: "One server's tool list and status.", args: map[string]ArgSpec{"id": str(true, "server id")}, handler: (*Runtime).mcpToolsImpl},
		{name: "mcp.add", description: "Register an MCP server (needs full-access).", args: map[string]ArgSpec{
			"id":        str(true, "server id, [A-Za-z0-9_-]+"),
			"transport": str(true, "http or stdio"),
			"url":       str(false, "server URL for http transport"),
			"command":   str(false, "command for stdio transport"),
			"args":      array(false, "stdio command arguments"),
			"env":       object(false, "stdio environment variables"),
			"label":     str(false, "display label"),
			"auth":      str(false, "none, static or oauth2; tokens are managed in Settings"),
			"enabled":   boolean(false, "start enabled"),
		}, handler: (*Runtime).mcpAddImpl},
		{name: "mcp.update", description: "Change an MCP server's config; absent keys keep their stored value (needs full-access).", args: map[string]ArgSpec{
			"id":             str(true, "server id"),
			"label":          str(false, "display label"),
			"url":            str(false, "server URL"),
			"command":        str(false, "stdio command"),
			"args":           array(false, "stdio command arguments"),
			"env":            object(false, "stdio environment variables"),
			"auth":           str(false, "none, static or oauth2"),
			"tier_overrides": object(false, "map of tool name to read/write/exec"),
			"enabled":        boolean(false, "enable or disable the server"),
		}, handler: (*Runtime).mcpUpdateImpl},
		{name: "mcp.remove", description: "Disconnect and delete an MCP server's config (needs full-access).", args: map[string]ArgSpec{"id": str(true, "server id")}, handler: (*Runtime).mcpRemoveImpl},
		{name: "mcp.connect", description: "Connect an MCP server and wait briefly for it to settle.", args: map[string]ArgSpec{"id": str(true, "server id")}, handler: (*Runtime).mcpConnectImpl},
		{name: "mcp.disconnect", description: "Close an MCP server's live session without touching its config.", args: map[string]ArgSpec{"id": str(true, "server id")}, handler: (*Runtime).mcpDisconnectImpl},
		{name: "mcp.reset_auth", description: "Drop a server's stored credential so the next connect signs in afresh.", args: map[string]ArgSpec{"id": str(true, "server id")}, handler: (*Runtime).mcpResetAuthImpl},
		{name: "mcp.call", description: "Run one tool on an MCP server directly.", args: map[string]ArgSpec{
			"id":   str(true, "server id"),
			"tool": str(true, "tool name"),
			"args": object(false, "tool arguments"),
		}, handler: (*Runtime).mcpCallImpl},
	}
}

// kernelMethods renders the catalog as jskernel methods. Every call is
// wrapped in OnlyArgs so a typo'd key fails with the accepted names instead
// of being silently ignored.
func kernelMethods(r *Runtime) []jskernel.Method {
	catalog := consoleCatalog()
	methods := make([]jskernel.Method, 0, len(catalog))
	for _, def := range catalog {
		def := def
		allowed := def.argNames()
		methods = append(methods, jskernel.Method{
			Name: def.name,
			Call: func(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
				if err := jskernel.OnlyArgs(args, allowed...); err != nil {
					return nil, err
				}
				return def.handler(r, ctx, args)
			},
		})
	}
	return methods
}

// findMethod resolves a user-supplied name ("sessions.create" or
// "console.sessions.create") to its catalog entry.
func findMethod(name string) (methodDef, bool) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "console.")
	for _, def := range consoleCatalog() {
		if def.name == name {
			return def, true
		}
	}
	return methodDef{}, false
}

// argSummary renders a method's arguments compactly for help listings:
// "session_id*, prompt, model?".
func (d methodDef) argSummary() string {
	if len(d.args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(d.args))
	for _, name := range d.argNames() {
		if d.args[name].Required {
			parts = append(parts, name+"*")
		} else {
			parts = append(parts, name+"?")
		}
	}
	return strings.Join(parts, ", ")
}

// example renders a minimal call for describe: required args filled with a
// placeholder of their type.
func (d methodDef) example() string {
	var b strings.Builder
	b.WriteString("console.")
	b.WriteString(d.name)
	b.WriteString("({")
	first := true
	for _, name := range d.argNames() {
		spec := d.args[name]
		if !spec.Required {
			continue
		}
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(name)
		b.WriteString(": ")
		switch spec.Type {
		case "integer":
			b.WriteString("0")
		case "boolean":
			b.WriteString("true")
		case "object":
			b.WriteString("{}")
		case "array":
			b.WriteString("[]")
		default:
			b.WriteString("\"…\"")
		}
	}
	b.WriteString("})")
	return b.String()
}

// helpHandler implements console.help: keyword search over the catalog, or a
// namespace listing when only namespace is given.
func helpHandler(_ *Runtime, _ context.Context, args map[string]any) (*jskernel.Result, error) {
	query, err := optionalString(args, "query")
	if err != nil {
		return nil, err
	}
	namespace, err := optionalString(args, "namespace")
	if err != nil {
		return nil, err
	}

	var matches []methodDef
	for _, def := range consoleCatalog() {
		if namespace != "" && def.namespace() != namespace {
			continue
		}
		if query == "" {
			matches = append(matches, def)
			continue
		}
		if scoreMethod(def, query) > 0 {
			matches = append(matches, def)
		}
	}
	if len(matches) == 0 {
		return marshalResult(map[string]any{"matches": []any{}, "note": "no methods matched; try console.help() to list everything"})
	}
	// Best matches first: name hits outrank description hits.
	sort.SliceStable(matches, func(i, j int) bool {
		return scoreMethod(matches[i], query) > scoreMethod(matches[j], query)
	})
	// A bare listing returns everything; a query is capped so a broad
	// search stays readable.
	if query != "" && len(matches) > 15 {
		matches = matches[:15]
	}
	out := make([]map[string]any, 0, len(matches))
	for _, def := range matches {
		entry := map[string]any{
			"name":        "console." + def.name,
			"description": def.description,
		}
		if s := def.argSummary(); s != "" {
			entry["args"] = s
		}
		out = append(out, entry)
	}
	return marshalResult(map[string]any{"matches": out})
}

// scoreMethod rates a method against a whitespace-separated query. Every
// word must appear somewhere in the name, description or argument docs;
// name hits score highest.
func scoreMethod(def methodDef, query string) int {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return 1
	}
	name := strings.ToLower(def.name)
	desc := strings.ToLower(def.description)
	var argText strings.Builder
	for argName, spec := range def.args {
		argText.WriteString(strings.ToLower(argName))
		argText.WriteString(" ")
		argText.WriteString(strings.ToLower(spec.Doc))
		argText.WriteString(" ")
	}
	args := argText.String()

	score := 0
	for _, w := range words {
		switch {
		case strings.Contains(name, w):
			score += 3
		case strings.Contains(desc, w) || strings.Contains(args, w):
			score++
		default:
			return 0
		}
	}
	return score
}

// describeHandler implements console.describe: the full contract for one
// method, including per-argument docs and a ready-to-adapt example call.
func describeHandler(_ *Runtime, _ context.Context, args map[string]any) (*jskernel.Result, error) {
	name, err := stringArg(args, "name")
	if err != nil {
		return nil, err
	}
	def, ok := findMethod(name)
	if !ok {
		return nil, fmt.Errorf("unknown console method %q; call console.help({query: %q}) to find it", name, strings.TrimPrefix(name, "console."))
	}
	argList := make([]map[string]any, 0, len(def.args))
	for _, argName := range def.argNames() {
		spec := def.args[argName]
		argList = append(argList, map[string]any{
			"name":     argName,
			"type":     spec.Type,
			"required": spec.Required,
			"doc":      spec.Doc,
		})
	}
	return marshalResult(map[string]any{
		"name":        "console." + def.name,
		"description": def.description,
		"args":        argList,
		"example":     def.example(),
	})
}
