//! Tool-call naming shared by both clients: display labels, MCP name
//! splitting, and the predicates that classify a call by what it touches.
//!
//! Keys are the Go server's registered tool names
//! (`apps/server-go/internal/agent/tools/*.go`) plus the snake_case /
//! camelCase aliases older sessions and other providers emit.

/// `mcp__<server>__<tool>` -> `(server, tool)`; `None` when malformed.
/// Segments are server-sanitised to `[A-Za-z0-9_-]`, so splitting on the
/// double underscore is safe — single underscores never form one.
pub fn parse_mcp_tool_name(name: &str) -> Option<(&str, &str)> {
    let rest = name.strip_prefix("mcp__")?;
    let sep = rest.find("__")?;
    if sep == 0 {
        return None;
    }
    let server = &rest[..sep];
    let tool = &rest[sep + 2..];
    if server.is_empty() || tool.is_empty() {
        return None;
    }
    Some((server, tool))
}

/// The exact-match label for a built-in tool name, if it has one.
fn builtin_label(name: &str) -> Option<&'static str> {
    Some(match name {
        "readFile" | "read_file" => "Read File",
        "writeFile" | "write_file" => "Write File",
        "batchWrite" | "batch_write" => "Batch Write",
        "editFile" | "edit_file" | "str_replace" => "Edit File",
        "bash" | "shell" | "command" => "Run Command",
        "bashJob" | "bash_job" => "Bash Job",
        "grep" | "search_files" => "Search Code",
        "glob" | "list_files" => "Find Files",
        "listDir" | "list_dir" | "ls" => "List Directory",
        "fetch" | "webFetch" => "Fetch URL",
        "webSearch" | "web_search" => "Web Search",
        "browser" => "Browser",
        "visualize" => "Visualize",
        "subagent" => "Subagent",
        "ask" => "Ask Question",
        "askMany" => "Ask Questions",
        "todo" => "Todo",
        "memory" => "Memory",
        "readSkill" => "Read Skill",
        "loadTools" => "Load Tools",
        _ => return None,
    })
}

/// Display label for a tool call: a built-in label, `"<tool> (<server>)"`
/// for an MCP tool, or the raw name when it is neither.
pub fn tool_label(name: &str) -> String {
    if let Some(label) = builtin_label(name) {
        return label.to_owned();
    }
    parse_mcp_tool_name(name)
        .map(|(server, tool)| format!("{} ({})", tool.replace('_', " "), server))
        .unwrap_or_else(|| name.to_owned())
}

/// A file edit whose arguments carry `oldContent` / `newContent`.
pub fn is_edit_file(name: &str) -> bool {
    matches!(name, "editFile" | "edit_file" | "str_replace")
}

/// A whole-file write: only the new content is known.
pub fn is_write_file(name: &str) -> bool {
    matches!(
        name,
        "writeFile" | "write_file" | "batchWrite" | "batch_write"
    )
}

/// A call that reads file content into its result.
pub fn is_read_file(name: &str) -> bool {
    matches!(name, "readFile" | "read_file" | "view" | "Read")
}

/// Whether a row should show the target file's type icon instead of the
/// generic tool glyph. Search/list tools also carry a `path`, but there it is
/// only the search scope (often a directory), so they keep the tool glyph.
pub fn is_file_target_tool(name: &str) -> bool {
    is_read_file(name) || is_edit_file(name) || is_write_file(name)
}

/// A subagent call, whose result is prose meant to be read as markdown.
pub fn is_subagent(name: &str) -> bool {
    name == "subagent"
}
