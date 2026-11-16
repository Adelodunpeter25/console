package com.console.mobile.core.util

import androidx.compose.ui.graphics.vector.ImageVector
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Brain
import io.github.lyxnx.compose.ui.tablericons.outline.Sparkles
import io.github.lyxnx.compose.ui.tablericons.outline.ChartBar
import io.github.lyxnx.compose.ui.tablericons.outline.FilePencil
import io.github.lyxnx.compose.ui.tablericons.outline.FileText
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import io.github.lyxnx.compose.ui.tablericons.outline.ListCheck
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal
import io.github.lyxnx.compose.ui.tablericons.outline.Code
import io.github.lyxnx.compose.ui.tablericons.outline.Tool
import com.console.mobile.SharedNative
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

// Tool names are classified server-side (console.v1 ToolCall.icon); this
// maps the server's icon keys to glyphs.

/** Server icon keys (tools.ToolIconKey, classify.go) → glyphs. */
private val SERVER_KEY_ICONS: Map<String, ImageVector> = mapOf(
    "sparkle" to TablerIcons.Outline.Sparkles,
    "terminal" to TablerIcons.Outline.Terminal,
    "code" to TablerIcons.Outline.Code,
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
 * Icon for a tool call via the server-classified key. Old sessions
 * predating classification (no key) get the generic tool glyph.
 */
fun getToolIcon(call: ToolCall): ImageVector =
    SERVER_KEY_ICONS[call.icon] ?: TablerIcons.Outline.Tool

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
    SharedNative.toolSummary(call.name, call.arguments?.toString() ?: "{}", cwd)?.let { return it }
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
    if (call.name == "code") {
        s("code")?.let { codeArgumentSummary(it)?.let { return it } }
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

/** Extracts a concise one-line action summary from a Code Mode script. */
fun codeArgumentSummary(code: String): String? {
    val trimmed = code.trim()
    if (trimmed.isEmpty()) return null

    // 1. Shell commands: $`cmd` or $'cmd' or $("cmd")
    val shellMatch = Regex("""\$\s*(?:`([^`]+)`|['"]([^'"]+)['"]|\(\s*["'`]?([^"'`]+)["'`]?\s*\))""").find(trimmed)
    if (shellMatch != null) {
        val cmd = shellMatch.groupValues.drop(1).firstOrNull { it.isNotBlank() }?.trim()
        if (!cmd.isNullOrEmpty()) {
            return truncate("$ $cmd", 72)
        }
    }

    // 2. Common tool helper functions: file("..."), edit("..."), etc.
    val helperMatch = Regex("""\b(file|write|edit|glob|grep|fetch|webSearch|webFetch|todo|readChat|readSkill)\s*\(\s*["']([^"']+)["']""").find(trimmed)
    if (helperMatch != null) {
        val fnName = helperMatch.groupValues[1]
        val arg = helperMatch.groupValues[2].trim()
        return truncate("$fnName($arg)", 72)
    }

    // 3. Subkernel calls: browser.open, console.ports.list, etc.
    val subkernelMatch = Regex("""\b(browser\.[a-zA-Z0-9_]+|console\.[a-zA-Z0-9_.]+)""").find(trimmed)
    if (subkernelMatch != null) {
        return truncate(subkernelMatch.groupValues[1], 72)
    }

    // 4. First non-empty non-comment line
    for (rawLine in trimmed.lineSequence()) {
        val line = rawLine.trim()
        if (line.isEmpty() || line.startsWith("//") || line.startsWith("/*") || line.startsWith("*")) continue
        val cleaned = line.removePrefix("const ")
            .removePrefix("let ")
            .removePrefix("var ")
            .removePrefix("return ")
            .removePrefix("await ")
            .trim()
        if (cleaned.isNotEmpty()) {
            return truncate(singleLine(cleaned), 72)
        }
    }
    return null
}
