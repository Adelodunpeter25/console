package com.console.mobile.core.util

import androidx.compose.ui.graphics.vector.ImageVector
import com.composables.icons.lucide.Brain
import com.composables.icons.lucide.BookOpen
import com.composables.icons.lucide.CircleHelp
import com.composables.icons.lucide.FilePen
import com.composables.icons.lucide.FilePlus
import com.composables.icons.lucide.FileText
import com.composables.icons.lucide.Files
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
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

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
    "glob" to Lucide.FolderSearch, "list_files" to TablerIcons.Outline.FolderOpen,
    "list_dir" to TablerIcons.Outline.FolderOpen, "listDir" to TablerIcons.Outline.FolderOpen, "ls" to TablerIcons.Outline.FolderOpen,
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

/** One-line argument summary for a tool row, mirroring the desktop's
 * `argument_summary` in apps/desktop/crates/console-ui/src/chat/toolcalls.rs:
 * target path first, then bashJob / memory specifics, then the first of
 * command/pattern/query/url/directory, then arg-array counts. */
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

/** Collapse every whitespace run (newlines, tabs, spaces) to single spaces so
 * a summary always fits on one line. */
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

// --- File tool classification and diff extraction -------------------------
// Ports of the file-tool branches in the desktop's
// apps/desktop/crates/console-ui/src/chat/toolcalls.rs and
// console-core/src/utils/diff.rs (extract_edit_args / extract_write_files /
// file_call_diffs), so a tool call renders the same on both clients.

/** An edit that carries both sides of the change (`oldContent` / `newContent`). */
fun isEditFileTool(name: String): Boolean = name in setOf("editFile", "edit_file", "str_replace")

/** A whole-file write — only the new content is known, so it diffs against "". */
fun isWriteFileTool(name: String): Boolean = name in setOf("writeFile", "write_file", "batchWrite", "batch_write")

/** A call that reads file content into its result. */
fun isReadFileTool(name: String): Boolean = name in setOf("read_file", "readFile", "view", "Read")

/** Whether a row for this tool should show the target file's type icon rather
 * than the generic tool glyph. Search/list tools also carry a `path`, but there
 * it is only the search scope (often a directory) — those keep the tool glyph. */
fun isFileTargetTool(name: String): Boolean =
    isReadFileTool(name) || isEditFileTool(name) || isWriteFileTool(name)

/** Whether the result of this call is prose meant to be read as markdown. */
fun isSubagentTool(name: String): Boolean = name == "subagent"

/** The target path of a file-oriented call, across the argument aliases. */
fun argumentPath(call: ToolCall): String? {
    val obj = call.arguments as? JsonObject ?: return null
    for (key in listOf("path", "filePath", "targetFile", "absolutePath")) {
        (obj[key] as? JsonPrimitive)?.takeIf { it.isString }?.let { return it.content }
    }
    return null
}

private fun jsonString(obj: JsonObject, key: String): String? =
    (obj[key] as? JsonPrimitive)?.takeIf { it.isString }?.content

/** (oldContent, newContent) for an `editFile`-shaped call. */
fun extractEditArgs(call: ToolCall): Pair<String, String>? {
    val obj = call.arguments as? JsonObject ?: return null
    val old = jsonString(obj, "oldContent") ?: return null
    val new = jsonString(obj, "newContent") ?: return null
    return old to new
}

/** Every `(path, content)` pair a whole-file write targets, in call order. A
 * `writeFile` carries one `path`/`content`; a `batchWrite` carries a `files`
 * array. Entries missing either half are skipped so one malformed element does
 * not blank the whole batch. */
fun extractWriteFiles(call: ToolCall): List<Pair<String, String>> {
    val obj = call.arguments as? JsonObject ?: return emptyList()
    (obj["files"] as? JsonArray)?.let { files ->
        return files.mapNotNull { entry ->
            val file = entry as? JsonObject ?: return@mapNotNull null
            val path = jsonString(file, "path") ?: return@mapNotNull null
            val content = jsonString(file, "content") ?: return@mapNotNull null
            path to content
        }
    }
    val path = listOf("path", "filePath", "targetFile").firstNotNullOfOrNull { jsonString(obj, it) }
        ?: return emptyList()
    val content = jsonString(obj, "content") ?: return emptyList()
    return listOf(path to content)
}

/** First written file — a display fallback for single-path labels only. */
fun extractWriteArgs(call: ToolCall): Pair<String, String>? = extractWriteFiles(call).firstOrNull()

/** One file's diff inside a tool call. */
data class FileCallDiff(val path: String, val diff: DiffResult)

/** Per-file diffs a file tool call describes, in call order. `editFile` yields
 * exactly one; whole-file writes yield one per file, diffed against an empty
 * file because the arguments carry no prior content. Non-file tools yield
 * nothing, so callers can test for emptiness. */
fun fileCallDiffs(call: ToolCall): List<FileCallDiff> {
    if (isEditFileTool(call.name)) {
        val (old, new) = extractEditArgs(call) ?: return emptyList()
        return listOf(FileCallDiff(argumentPath(call).orEmpty(), computeLineDiff(old, new)))
    }
    if (isWriteFileTool(call.name)) {
        return extractWriteFiles(call).map { (path, content) ->
            FileCallDiff(path, computeLineDiff("", content))
        }
    }
    return emptyList()
}

private val LINE_NUMBER_PREFIX = Regex("""^(\s*)(\d+): ?(.*)$""")

/** Split read-file output into `(gutter numbers, code body)`. Drops a leading
 * `File:` metadata header, and reuses the tool's own line numbers when most
 * lines carry a `123: ` prefix (the server truncates long reads, so the printed
 * numbers are not always 1..n). */
fun parseReadFileOutput(raw: String): Pair<List<String>, List<String>> {
    val lines = raw.lines()
    val headerEnd = if (lines.firstOrNull()?.startsWith("File:") == true) {
        val firstBlank = (1 until lines.size).firstOrNull { lines[it].isEmpty() } ?: -1
        if (firstBlank in 1..6) firstBlank + 1 else 0
    } else {
        0
    }
    val codeLines = lines.drop(headerEnd)
    val parsed = codeLines.map { LINE_NUMBER_PREFIX.matchEntire(it) }
    if (parsed.count { it != null } * 2 > codeLines.size) {
        val numbers = parsed.map { m -> m?.groupValues?.get(2).orEmpty() }
        val body = parsed.map { m -> m?.groupValues?.get(3) ?: "" }
        return numbers to body
    }
    return codeLines.indices.map { (it + 1).toString() } to codeLines
}
