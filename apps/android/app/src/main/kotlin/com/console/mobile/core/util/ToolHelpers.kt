package com.console.mobile.core.util

import androidx.compose.ui.graphics.vector.ImageVector
import com.composables.icons.lucide.Brain
import com.composables.icons.lucide.BookOpen
import com.composables.icons.lucide.CircleHelp
import com.composables.icons.lucide.FilePen
import com.composables.icons.lucide.FilePlus
import com.composables.icons.lucide.FileText
import com.composables.icons.lucide.Files
import com.composables.icons.lucide.Folder
import com.composables.icons.lucide.FolderSearch
import com.composables.icons.lucide.Globe
import com.composables.icons.lucide.ListTodo
import com.composables.icons.lucide.Lucide
import com.composables.icons.lucide.MessagesSquare
import com.composables.icons.lucide.Search
import com.composables.icons.lucide.SquareTerminal
import com.composables.icons.lucide.Terminal
import com.composables.icons.lucide.Users
import com.composables.icons.lucide.Wrench
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

val EXT_LANG_MAP: Map<String, String> = mapOf(
    "ts" to "typescript", "tsx" to "typescript", "js" to "javascript", "jsx" to "javascript",
    "mjs" to "javascript", "cjs" to "javascript", "json" to "json", "html" to "html",
    "css" to "css", "scss" to "scss", "py" to "python", "rb" to "ruby", "go" to "go",
    "rs" to "rust", "java" to "java", "kt" to "kotlin", "swift" to "swift",
    "c" to "c", "h" to "c", "cpp" to "cpp", "hpp" to "cpp", "cs" to "csharp",
    "php" to "php", "sh" to "bash", "bash" to "bash", "zsh" to "bash",
    "yml" to "yaml", "yaml" to "yaml", "toml" to "toml", "xml" to "xml",
    "md" to "markdown", "markdown" to "markdown", "sql" to "sql", "lua" to "lua",
    "r" to "r", "dart" to "dart", "vue" to "html", "svelte" to "html",
    "graphql" to "graphql", "dockerfile" to "dockerfile"
)

fun langFromPath(filePath: String): String? {
    val basename = filePath.split('/').lastOrNull().orEmpty()
    if (basename == "Dockerfile") return "dockerfile"
    if (basename == "Makefile") return "makefile"
    val ext = basename.substringAfterLast('.', "").lowercase()
    if (ext.isEmpty()) return null
    return EXT_LANG_MAP[ext]
}

// Keys must match the Go server's registered tool names verbatim
// (apps/server-go/internal/agent/tools/*.go `NewTool("...")` calls) — these are
// what actually reach the wire in ToolCall.Name / ToolResult.ToolName, not a
// camelCase convention. The camelCase / snake_case aliases mirror the desktop
// app's `call_label` in apps/desktop/crates/console-ui/src/chat/toolcalls.rs
// so both clients render the same label for the same call.
val TOOL_LABELS: Map<String, String> = mapOf(
    "read_file" to "Read File", "readFile" to "Read File",
    "write_file" to "Write File", "writeFile" to "Write File",
    "batchWrite" to "Batch Write", "batch_write" to "Batch Write",
    "editFile" to "Edit File", "edit_file" to "Edit File", "str_replace" to "Edit File",
    "bash" to "Run Command", "shell" to "Run Command", "command" to "Run Command",
    "bashJob" to "Bash Job", "bash_job" to "Bash Job",
    "grep" to "Search Code", "search_files" to "Search Code",
    "glob" to "Find Files", "list_files" to "Find Files",
    "list_dir" to "List Directory", "listDir" to "List Directory", "ls" to "List Directory",
    "webFetch" to "Fetch URL", "fetch" to "Fetch URL",
    "webSearch" to "Web Search", "web_search" to "Web Search",
    "browser" to "Browser",
    "subagent" to "Subagent",
    "ask" to "Ask Question", "askMany" to "Ask Questions", "todo" to "Todo",
    "memory" to "Memory", "readSkill" to "Read Skill",
)

fun getToolLabel(name: String): String = TOOL_LABELS[name] ?: name

// Mirrors the desktop's `activity_icon` alias handling in
// apps/desktop/crates/console-ui/src/primitives/mod.rs.
val TOOL_ICONS: Map<String, ImageVector> = mapOf(
    "read_file" to Lucide.FileText, "readFile" to Lucide.FileText,
    "write_file" to Lucide.FilePlus, "writeFile" to Lucide.FilePlus,
    "batchWrite" to Lucide.Files, "batch_write" to Lucide.Files,
    "editFile" to Lucide.FilePen, "edit_file" to Lucide.FilePen, "str_replace" to Lucide.FilePen,
    "bash" to Lucide.Terminal, "shell" to Lucide.Terminal, "command" to Lucide.Terminal,
    "bashJob" to Lucide.SquareTerminal, "bash_job" to Lucide.SquareTerminal,
    "grep" to Lucide.Search, "search_files" to Lucide.Search,
    "glob" to Lucide.FolderSearch, "list_files" to Lucide.Folder,
    "list_dir" to Lucide.Folder, "listDir" to Lucide.Folder, "ls" to Lucide.Folder,
    "webFetch" to Lucide.Globe, "fetch" to Lucide.Globe,
    "webSearch" to Lucide.Globe, "web_search" to Lucide.Globe, "browser" to Lucide.Globe,
    "subagent" to Lucide.Users,
    "ask" to Lucide.CircleHelp, "askMany" to Lucide.MessagesSquare, "todo" to Lucide.ListTodo,
    "memory" to Lucide.Brain, "readSkill" to Lucide.BookOpen,
)

fun getToolIcon(name: String): ImageVector = TOOL_ICONS[name] ?: Lucide.Wrench

fun formatUnknown(v: Any?): String = when (v) {
    null -> "null"
    is String -> v
    else -> v.toString()
}

fun getFileName(filePath: String?): String {
    if (filePath.isNullOrEmpty()) return ""
    val clean = filePath.replace('\\', '/')
    return clean.split('/').lastOrNull().orEmpty().ifEmpty { filePath }
}

fun toRelativePath(filePath: String?, cwd: String?): String {
    if (filePath.isNullOrEmpty()) return ""
    val p = filePath.replace('\\', '/')
    val c = cwd?.replace('\\', '/')?.trimEnd('/')
    if (!c.isNullOrEmpty() && (p == c || p.startsWith("$c/"))) {
        return if (p == c) "." else p.substring(c.length + 1)
    }
    return p
}

/// One-line argument summary for a tool row, mirroring the desktop's
/// `argument_summary` in apps/desktop/crates/console-ui/src/chat/toolcalls.rs:
/// target path first, then bashJob / memory specifics, then the first of
/// command/pattern/query/url/directory, then arg-array counts.
fun toolCallSummary(call: ToolCall, cwd: String? = null): String? {
    val args = call.arguments as? JsonObject ?: return null
    fun s(key: String): String? = (args[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

    for (k in listOf("path", "filePath", "targetFile", "absolutePath")) {
        val v = s(k)
        if (v != null) return truncate(toRelativePath(v, cwd ?: s("cwd")), 72)
    }
    if (call.name == "bashJob" || call.name == "bash_job") {
        val action = s("action") ?: "status"
        val job = s("jobId")
        return truncate(if (job != null) "$action $job" else action, 72)
    }
    if (call.name == "memory") {
        val op = s("op") ?: "memory"
        s("query")?.let { return truncate("$op \"$it\"", 72) }
        s("content")?.let { return truncate("$op \"${singleLine(it)}\"", 72) }
        (args["tags"] as? JsonArray)
            ?.mapNotNull { (it as? JsonPrimitive)?.takeIf { p -> p.isString }?.content }
            ?.takeIf { it.isNotEmpty() }
            ?.let { return truncate("$op [${it.joinToString(", ")}]", 72) }
        s("id")?.let { return truncate("$op $it", 72) }
        return truncate(op, 72)
    }
    // Multi-line values (heredoc scripts, multi-line queries) must collapse to
    // one line: a literal newline renders as a hard line break that no
    // truncation can suppress. `directory` renders relative to the cwd.
    for (k in listOf("command", "pattern", "query", "url", "directory", "question")) {
        val v = s(k) ?: continue
        val value = if (k == "directory") toRelativePath(v, cwd ?: s("cwd")) else v
        return truncate(singleLine(value), 72)
    }
    for (k in listOf("paths", "files", "operations")) {
        val arr = args[k] as? JsonArray
        if (arr != null && arr.isNotEmpty()) {
            return "${arr.size} ${if (k == "operations") "operations" else "files"}"
        }
    }
    return null
}

/// Collapse every whitespace run (newlines, tabs, spaces) to single spaces so
/// a summary always fits on one line.
fun singleLine(value: String): String =
    value.split(Regex("\\s+")).filter { it.isNotEmpty() }.joinToString(" ")

fun truncate(value: String, maxChars: Int): String =
    if (value.length <= maxChars) value else value.take(maxChars) + "…"

fun resultText(result: ToolResult): String {
    val c = result.content ?: return "null"
    if (c is JsonPrimitive) return if (c.isString) c.content else c.toString()
    if (c is JsonObject) {
        val text = (c["text"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        if (text != null) return text
        val inner = c["content"]
        if (inner != null && inner != c) return resultText(result.copy(content = inner))
        return c.toString()
    }
    if (c is JsonArray) {
        return c.joinToString("\n") { el ->
            when {
                el is JsonPrimitive && el.isString -> el.content
                el is JsonObject && (el["type"] as? JsonPrimitive)?.content == "text" ->
                    (el["text"] as? JsonPrimitive)?.takeIf { it.isString }?.content.orEmpty()
                else -> ""
            }
        }
    }
    return c.toString()
}
