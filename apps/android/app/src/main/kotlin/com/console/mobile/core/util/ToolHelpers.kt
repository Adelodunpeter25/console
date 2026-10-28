package com.console.mobile.core.util

import androidx.compose.ui.graphics.vector.ImageVector
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Book
import io.github.lyxnx.compose.ui.tablericons.outline.Brain
import io.github.lyxnx.compose.ui.tablericons.outline.Sparkles
import io.github.lyxnx.compose.ui.tablericons.outline.ChartBar
import io.github.lyxnx.compose.ui.tablericons.outline.FilePencil
import io.github.lyxnx.compose.ui.tablericons.outline.FilePlus
import io.github.lyxnx.compose.ui.tablericons.outline.FileText
import io.github.lyxnx.compose.ui.tablericons.outline.Files
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.FolderSearch
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import io.github.lyxnx.compose.ui.tablericons.outline.HelpCircle
import io.github.lyxnx.compose.ui.tablericons.outline.ListCheck
import io.github.lyxnx.compose.ui.tablericons.outline.Messages
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2
import io.github.lyxnx.compose.ui.tablericons.outline.Tool
import io.github.lyxnx.compose.ui.tablericons.outline.Users
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

// Mirrors the desktop's `activity_icon` alias handling in
// apps/desktop/crates/console-ui/src/primitives/mod.rs.
val TOOL_ICONS: Map<String, ImageVector> = mapOf(
    "read_file" to TablerIcons.Outline.FileText, "readFile" to TablerIcons.Outline.FileText,
    "write_file" to TablerIcons.Outline.FilePlus, "writeFile" to TablerIcons.Outline.FilePlus,
    "batchWrite" to TablerIcons.Outline.Files, "batch_write" to TablerIcons.Outline.Files,
    "editFile" to TablerIcons.Outline.FilePencil, "edit_file" to TablerIcons.Outline.FilePencil, "str_replace" to TablerIcons.Outline.FilePencil,
    "bash" to TablerIcons.Outline.Terminal, "shell" to TablerIcons.Outline.Terminal, "command" to TablerIcons.Outline.Terminal,
    "bashJob" to TablerIcons.Outline.Terminal2, "bash_job" to TablerIcons.Outline.Terminal2,
    "grep" to TablerIcons.Outline.Search, "search_files" to TablerIcons.Outline.Search,
    "glob" to TablerIcons.Outline.FolderSearch, "list_files" to TablerIcons.Outline.FolderOpen,
    "list_dir" to TablerIcons.Outline.FolderOpen, "listDir" to TablerIcons.Outline.FolderOpen, "ls" to TablerIcons.Outline.FolderOpen,
    "webFetch" to TablerIcons.Outline.Globe, "fetch" to TablerIcons.Outline.Globe,
    "webSearch" to TablerIcons.Outline.Globe, "web_search" to TablerIcons.Outline.Globe, "browser" to TablerIcons.Outline.Globe,
    "visualize" to TablerIcons.Outline.ChartBar,
    "subagent" to TablerIcons.Outline.Users,
    "ask" to TablerIcons.Outline.HelpCircle, "askMany" to TablerIcons.Outline.Messages, "todo" to TablerIcons.Outline.ListCheck,
    "memory" to TablerIcons.Outline.Brain, "readSkill" to TablerIcons.Outline.Book,
)

fun getToolIcon(name: String): ImageVector {
    TOOL_ICONS[name]?.let { return it }
    // The loader and every namespaced MCP tool share the plug glyph — dynamic
    // `mcp__<server>__<tool>` names can never be map keys.
    if (name == "loadTools" || name.startsWith("mcp__")) return TablerIcons.Outline.PlugConnected
    return TablerIcons.Outline.Tool
}

/** Server icon keys (console.v1 ToolCall.icon) → glyphs. */
private val SERVER_KEY_ICONS: Map<String, ImageVector> = mapOf(
    "sparkle" to TablerIcons.Outline.Sparkles,
    "terminal" to TablerIcons.Outline.Terminal,
    "terminal2" to TablerIcons.Outline.Terminal2,
    "pencil" to TablerIcons.Outline.FilePencil,
    "file" to TablerIcons.Outline.FileText,
    "folder" to TablerIcons.Outline.FolderOpen,
    "search" to TablerIcons.Outline.Search,
    "globe" to TablerIcons.Outline.Globe,
    "chart" to TablerIcons.Outline.ChartBar,
    "list" to TablerIcons.Outline.ListCheck,
    "memory" to TablerIcons.Outline.Brain,
    "package" to TablerIcons.Outline.PlugConnected,
)

/**
 * Icon for a tool call: the server-classified key when the call carries one
 * (console.v1 ToolCall.icon), else the legacy name table.
 */
fun getToolIcon(call: ToolCall): ImageVector {
    call.icon?.let { SERVER_KEY_ICONS[it]?.let { v -> return v } }
    return getToolIcon(call.name)
}

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
    // The loader takes `{group: "mcp:<server-id>"}` — surface it verbatim so
    // the row says which group was loaded.
    if (call.name == "loadTools") {
        return s("group")?.let { truncate(it, 72) }
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

// Tool labels, the edit/write/read classification and the per-file diffs now
// come from `console-shared` through `SharedNative`; only Compose icons and the
// argument summaries below are Android-specific.

/** The target path of a file-oriented call, across the argument aliases. */
fun argumentPath(call: ToolCall): String? {
    val obj = call.arguments as? JsonObject ?: return null
    for (key in listOf("path", "filePath", "targetFile", "absolutePath")) {
        (obj[key] as? JsonPrimitive)?.takeIf { it.isString }?.let { return it.content }
    }
    return null
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
