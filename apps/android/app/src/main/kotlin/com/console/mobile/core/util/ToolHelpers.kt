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

