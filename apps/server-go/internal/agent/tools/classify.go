// Tool-call presentation classification, derived once server-side so the
// clients render labels, icons and kinds instead of each keeping its own
// table of tool names (see proto/console/v1/message.proto ToolCall).
//
// The registered names are the Go server's tools (./tool.go Name values);
// the snake_case / camelCase aliases cover older sessions and names other
// providers emit. MCP tools are `mcp__<server>__<tool>`: segments are
// sanitised to [A-Za-z0-9_-] server-side, so splitting on the double
// underscore is safe — single underscores never form one.
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
	case "readFile", "read_file":
		return "Read File"
	case "writeFile", "write_file":
		return "Write File"
	case "batchWrite", "batch_write":
		return "Batch Write"
	case "editFile", "edit_file", "str_replace":
		return "Edit File"
	case "bash", "shell", "command":
		return "Run Command"
	case "bashJob", "bash_job":
		return "Bash Job"
	case "grep", "search_files":
		return "Search Code"
	case "glob", "list_files":
		return "Find Files"
	case "listDir", "list_dir", "ls":
		return "List Directory"
	case "fetch", "webFetch":
		return "Fetch URL"
	case "webSearch", "web_search":
		return "Web Search"
	case "browser":
		return "Browser"
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
	case "readSkill":
		return "Read Skill"
	case "loadTools":
		return "Load Tools"
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
// `"<tool> (<server>)"` for an MCP tool, or the raw name when it is
// neither. Mirrors console-shared's tool_label (desktop/Android fallback
// for pre-classification sessions).
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
	case "editFile", "edit_file", "str_replace":
		return true
	}
	return false
}

// IsWriteFile reports a whole-file write: only the new content is known.
func IsWriteFile(name string) bool {
	switch name {
	case "writeFile", "write_file", "batchWrite", "batch_write":
		return true
	}
	return false
}

// IsReadFile reports a call that reads file content into its result.
func IsReadFile(name string) bool {
	switch name {
	case "readFile", "read_file", "view", "Read":
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
	case "reasoning", "think":
		return "sparkle"
	case "bash", "shell", "command", "bashJob", "bash_job":
		return "terminal"
	case "edit_file", "editFile", "write_file", "writeFile", "str_replace":
		return "pencil"
	case "read_file", "readFile":
		return "file"
	case "list_files", "list_dir", "listDir", "ls":
		return "folder"
	case "search_files", "grep", "glob", "find", "findFile", "find_file":
		return "search"
	case "web_search", "webSearch", "browser":
		return "globe"
	case "visualize":
		return "chart"
	case "plan":
		return "list"
	case "memory":
		return "memory"
	case "loadTools":
		return "package"
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
