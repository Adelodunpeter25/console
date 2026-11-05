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
	"fmt"
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
	switch name {
	case "read_file":
		return "Read File"
	case "write_file":
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
	case "list_dir":
		return "List Directory"
	case "webFetch":
		return "Fetch URL"
	case "webSearch":
		return "Web Search"
	case "browser":
		return "Browser"
	case "browser_reset":
		return "Reset Browser"
	case "visualize":
		return "Visualize"
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
	case "read_chat":
		return "Read Chat"
	case "readSkill":
		return "Read Skill"
	case "loadTools":
		return "Load Tools"
	case "console":
		return "Console"
	case "console_reset":
		return "Reset Console"
	case "computer":
		return "Computer"
	case "computer_reset":
		return "Reset Computer"
	case "writePlan":
		return "Write Plan"
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

// IsEditFile reports a file edit whose arguments carry oldContent/newContent.
func IsEditFile(name string) bool {
	switch name {
	case "editFile":
		return true
	}
	return false
}

// IsWriteFile reports a whole-file write: only the new content is known.
func IsWriteFile(name string) bool {
	switch name {
	case "write_file", "batchWrite":
		return true
	}
	return false
}

// IsReadFile reports a call that reads file content into its result.
func IsReadFile(name string) bool {
	switch name {
	case "read_file":
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
func IsSubagent(name string) bool { return name == "subagent" }

// ToolIconKey is the icon key from the server's activity-icon table
// (sparkle, terminal, pencil, file, folder, search, globe, chart, list,
// memory, package, wrench). Clients map keys to their own glyphs.
func ToolIconKey(name string) string {
	if _, _, ok := ParseMCPToolName(name); ok {
		return "package"
	}
	switch name {
	case "bash", "bashJob":
		return "terminal"
	case "editFile", "write_file", "batchWrite":
		return "pencil"
	case "read_file":
		return "file"
	case "list_dir":
		return "folder"
	case "grep", "glob":
		return "search"
	case "webSearch", "webFetch", "browser", "browser_reset":
		return "globe"
	case "visualize":
		return "chart"
	case "todo":
		return "list"
	case "memory", "read_chat":
		// Cross-session recall: reading another chat is the same kind
		// of act as recalling a stored memory.
		return "memory"
	case "loadTools":
		return "package"
	case "console", "console_reset":
		// A script that drives the app: the terminal glyph, like bash.
		return "terminal"
	case "computer", "computer_reset":
		return "globe"
	case "writePlan":
		return "file"
	default:
		return "wrench"
	}
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
