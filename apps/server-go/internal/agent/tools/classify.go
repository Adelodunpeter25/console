// Tool-call presentation classification, derived once server-side so the
// clients render labels, icons and kinds instead of each keeping its own
// table of tool names (see proto/console/v1/message.proto ToolCall).
//
// Only the names this server registers appear here. The server is the source of
// every tool name a client sees, so there are no aliases: a name that is not
// registered gets no label, no icon and no flags, and shows as its raw name,
// which is honest and visible. A test (tests/tools/classify_registry_test.go)
// checks every name in these tables against the real registry, so the tables
// cannot drift from it in either direction.
//
// MCP tools are `mcp__<server>__<tool>`: segments are sanitised to
// [A-Za-z0-9_-] server-side, so splitting on the double underscore is safe —
// single underscores never form one.
package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Kind flags for ToolCall.kind_flags (proto/console/v1/message.proto).
const (
	KindFlagEdit       = 1
	KindFlagWrite      = 2
	KindFlagRead       = 4
	KindFlagFileTarget = 8
	KindFlagSubagent   = 16
)

// builtinLabel is the exact-match label for a built-in tool name.
func builtinLabel(name string) string {
	switch CanonicalName(name) {
	case "readFile":
		return "Read File"
	case "writeFile":
		return "Write File"
	case "batchWrite":
		return "Batch Write"
	case "editFile":
		return "Edit File"
	case "bash":
		return "Run Command"
	case "bashJob":
		return "Bash Job"
	case "grep":
		return "Search Code"
	case "glob":
		return "Find Files"
	case "listDir":
		return "List Directory"
	case "webFetch":
		return "Fetch URL"
	case "webSearch":
		return "Web Search"
	case "browser":
		return "Browser"
	case "browserReset":
		return "Reset Browser"
	case "render":
		return "Render"
	case "preview":
		return "Preview"
	case "subagent":
		return "Subagent"
	case "ask":
		return "Ask Question"
	case "askMany":
		return "Ask Questions"
	case "todo":
		return "Todo"
	case "memory":
		return "Memory"
	case "readChat":
		return "Read Chat"
	case "readSkill":
		return "Read Skill"
	case "loadTools":
		return "Load Tools"
	case "console":
		return "Console"
	case "consoleReset":
		return "Reset Console"
	case "computer":
		return "Computer"
	case "computerReset":
		return "Reset Computer"
	case "writePlan":
		return "Write Plan"
	case "requestFullAccess":
		return "Request Full Access"
	case "code":
		return "Code"
	case "codeReset":
		return "Reset Code"
	default:
		return ""
	}
}

// ParseMCPToolName splits `mcp__<server>__<tool>`; ok=false when malformed.
func ParseMCPToolName(name string) (server, tool string, ok bool) {
	rest, found := strings.CutPrefix(name, "mcp__")
	if !found {
		return "", "", false
	}
	server, tool, found = strings.Cut(rest, "__")
	if !found || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

// ToolLabel renders the display label for a tool call: a built-in label,
// `"<tool> (<server>)"` for an MCP tool, or the raw name when it is neither.
func ToolLabel(name string) string {
	if label := builtinLabel(name); label != "" {
		return label
	}
	if server, tool, ok := ParseMCPToolName(name); ok {
		return fmt.Sprintf("%s (%s)", strings.ReplaceAll(tool, "_", " "), server)
	}
	return name
}

var shellCmdRegex = regexp.MustCompile("(?s)\\$\\s*(?:`([^`]+)`|['\"]([^'\"]+)['\"]|\\(\\s*[\"'`]([^\"'`]+)[\"'`]\\s*\\))")

var codeHelperNames = []string{
	"fileRaw", "readRaw", "file", "write", "edit", "glob", "grep", "list",
	"fetch", "webFetch", "webSearch", "todo", "readChat", "readSkill",
	"render", "preview", "ask", "askMany", "mcp", "tools", "store",
	"spawnContext", "help",
}

func isCodeIdent(b byte) bool {
	return (b >= 97 && b <= 122) || (b >= 65 && b <= 90) || (b >= 48 && b <= 57) || b == 95 || b == 36
}

func codeRootName(name string) bool {
	return name == "console" || name == "browser" || name == "computer"
}

func codeTruncate(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	r := []rune(value)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return value
}

func codeHelperLabel(name, rest string) string {
	rest = strings.TrimSpace(rest)
	if rest == "" { return name + "()" }
	if rest[0] == 34 || rest[0] == 39 || rest[0] == 96 {
		q := rest[0]
		if end := strings.IndexByte(rest[1:], q); end >= 0 {
			return name + "(" + codeTruncate(rest[1:1+end], 40) + ")"
		}
	}
	if end := strings.IndexAny(rest, ",)"); end >= 0 { rest = strings.TrimSpace(rest[:end]) }
	if rest != "" && isCodeIdent(rest[0]) {
		i := 0
		for i < len(rest) && isCodeIdent(rest[i]) { i++ }
		return name + "(" + codeTruncate(rest[:i], 40) + ")"
	}
	return name + "()"
}

func codeSkip(code string, i int) int {
	ch := code[i]
	if ch == 39 || ch == 34 || ch == 96 {
		j := i + 1
		for j < len(code) {
			if code[j] == 92 { j += 2; continue }
			if code[j] == ch { return j + 1 - i }
			j++
		}
		return len(code) - i
	}
	if ch == 47 && i+1 < len(code) && code[i+1] == 47 {
		j := i + 2
		for j < len(code) && code[j] != 10 { j++ }
		return j - i
	}
	if ch == 47 && i+1 < len(code) && code[i+1] == 42 {
		j := i + 2
		for j+1 < len(code) && !(code[j] == 42 && code[j+1] == 47) { j++ }
		if j+1 < len(code) { j += 2 }
		return j - i
	}
	return 0
}

func codeIsHelper(name string) bool {
	for _, h := range codeHelperNames { if h == name { return true } }
	return false
}
// codeScriptLabel names the script's real helper or subkernel calls.
// Strings and comments are skipped so a mention inside them is not a call.
func codeScriptLabel(code string) string {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" { return "" }
	if m := shellCmdRegex.FindStringSubmatch(trimmed); len(m) > 1 {
		for _, part := range m[1:] {
			if cmd := strings.TrimSpace(part); cmd != "" { return codeTruncate("$ "+cmd, 50) }
		}
	}
	helper := ""
	var roots []string
	seen := map[string]bool{}
	for i := 0; i < len(code); {
		if n := codeSkip(code, i); n > 0 { i += n; continue }
		if !isCodeIdent(code[i]) || (i > 0 && isCodeIdent(code[i-1])) { i++; continue }
		j := i
		for j < len(code) && isCodeIdent(code[j]) { j++ }
		name := code[i:j]
		if codeRootName(name) && j < len(code) && code[j] == 46 {
			p := j + 1
			for p < len(code) && (isCodeIdent(code[p]) || code[p] == 46) { p++ }
			z := p
			for z < len(code) && (code[z] == 32 || code[z] == 9 || code[z] == 10 || code[z] == 13) { z++ }
			if z < len(code) && code[z] == 40 {
				full := name + "." + strings.Trim(code[j+1:p], ".")
				if !seen[full] { seen[full] = true; roots = append(roots, full) }
				i = z
				continue
			}
		}
		k := j
		for k < len(code) && (code[k] == 32 || code[k] == 9 || code[k] == 10 || code[k] == 13) { k++ }
		if k < len(code) && code[k] == 40 && helper == "" && codeIsHelper(name) {
			helper = codeHelperLabel(name, code[k+1:])
		}
		i = j
	}
	if helper != "" { return helper }
	if len(roots) > 0 { return codeTruncate(strings.Join(roots, ", "), 50) }
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "*") { continue }
		for _, prefix := range []string{"const ", "let ", "var ", "return ", "await "} {
			line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
		if line != "" { return codeTruncate(line, 50) }
	}
	return ""
}
// codeScriptSummary extracts a concise one-line action label from a code tool's arguments.
func codeScriptSummary(rawArgs json.RawMessage) string {
	if len(rawArgs) == 0 { return "" }
	var in struct { Code string `json:"code"` }
	if err := json.Unmarshal(rawArgs, &in); err != nil || in.Code == "" { return "" }
	return codeScriptLabel(in.Code)
}
// ToolCallLabel renders the display label for a tool call, using its arguments
// (such as code scripts) when they carry descriptive details.
func ToolCallLabel(name string, arguments json.RawMessage) string {
	if name == "code" {
		if summary := codeScriptSummary(arguments); summary != "" {
			return summary
		}
		return "Code"
	}
	return ToolLabel(name)
}

// IsEditFile reports a file edit whose arguments carry oldContent/newContent.
func IsEditFile(name string) bool {
	switch CanonicalName(name) {
	case "editFile":
		return true
	}
	return false
}

// IsWriteFile reports a whole-file write: only the new content is known.
func IsWriteFile(name string) bool {
	switch CanonicalName(name) {
	case "writeFile", "batchWrite":
		return true
	}
	return false
}

// IsReadFile reports a call that reads file content into its result.
func IsReadFile(name string) bool {
	switch CanonicalName(name) {
	case "readFile":
		return true
	}
	return false
}

// IsFileTargetTool reports whether a row shows the target file's type icon
// instead of the generic tool glyph. Search/list tools also carry a path,
// but there it is only the search scope (often a directory), so they keep
// the tool glyph.
func IsFileTargetTool(name string) bool {
	return IsReadFile(name) || IsEditFile(name) || IsWriteFile(name)
}

// IsSubagent reports a subagent call, whose result is prose meant to be
// read as markdown.
func IsSubagent(name string) bool { return CanonicalName(name) == "subagent" }

// ToolIconKey is the icon key from the server's activity-icon table
// (sparkle, terminal, pencil, file, folder, search, globe, chart, list,
// memory, package, wrench). Clients map keys to their own glyphs.
func ToolIconKey(name string) string {
	if _, _, ok := ParseMCPToolName(name); ok {
		return "package"
	}
	switch CanonicalName(name) {
	case "bash", "bashJob":
		return "terminal"
	case "editFile", "writeFile", "batchWrite", "requestFullAccess":
		return "pencil"
	case "readFile":
		return "file"
	case "listDir":
		return "folder"
	case "grep", "glob":
		return "search"
	case "webSearch", "webFetch", "browser", "browserReset":
		return "globe"
	case "render", "preview":
		return "chart"
	case "todo":
		return "list"
	case "memory", "readChat":
		// Cross-session recall: reading another chat is the same kind
		// of act as recalling a stored memory.
		return "memory"
	case "loadTools":
		return "package"
	case "console", "consoleReset":
		// A script that drives the app: the terminal glyph, like bash.
		return "terminal"
	case "computer", "computerReset":
		return "globe"
	case "code", "codeReset":
		return "code"
	case "writePlan":
		return "file"
	default:
		return "wrench"
	}
}

// ToolCallSummary derives the one-line detail shown beside a tool's label.
// It mirrors the argument summaries previously computed by clients so the
// server is the single source of truth for tool-call presentation.
func ToolCallSummary(name string, arguments json.RawMessage) string {
	if len(arguments) == 0 { return "" }
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &obj); err != nil { return "" }
	str := func(keys ...string) string {
		for _, k := range keys {
			if raw, ok := obj[k]; ok {
				var s string
				if json.Unmarshal(raw, &s) == nil && s != "" { return s }
			}
		}
		return ""
	}
	if s := str("path", "filePath", "targetFile", "absolutePath"); s != "" { return codeTruncate(s, 72) }
	if name == "bashJob" || name == "bash_job" {
		action := str("action"); if action == "" { action = "status" }
		if id := str("jobId"); id != "" { return codeTruncate(action+" "+id, 72) }
		return action
	}
	if name == "memory" {
		op := str("op"); if op == "" { op = "memory" }
		if q := str("query"); q != "" { return codeTruncate(op+" \""+q+"\"", 72) }
		if c := str("content"); c != "" { return codeTruncate(op+" \""+codeTruncate(c, 60)+"\"", 72) }
		return op
	}
	if name == "loadTools" { return codeTruncate(str("group"), 72) }
	if name == "code" || name == "console" || name == "computer" {
		if code := str("code"); code != "" { return codeScriptLabel(code) }
	}
	for _, k := range []string{"command", "pattern", "query", "url", "directory", "question"} {
		if s := str(k); s != "" { return codeTruncate(s, 72) }
	}
	return ""
}
// ToolKindFlags packs the classification predicates into the wire bitmask.
func ToolKindFlags(name string) uint32 {
	var flags uint32
	if IsEditFile(name) {
		flags |= KindFlagEdit
	}
	if IsWriteFile(name) {
		flags |= KindFlagWrite
	}
	if IsReadFile(name) {
		flags |= KindFlagRead
	}
	if IsFileTargetTool(name) {
		flags |= KindFlagFileTarget
	}
	if IsSubagent(name) {
		flags |= KindFlagSubagent
	}
	return flags
}