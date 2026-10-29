package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.SharedNative
import com.console.mobile.core.util.argumentPath
import com.console.mobile.core.util.getToolIcon
import com.console.mobile.core.util.resultText
import com.console.mobile.core.util.toolCallSummary
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import com.console.mobile.feature.chat.markdown.CustomMarkdown
import com.console.mobile.ui.code.CodeViewer
import com.console.mobile.ui.code.languageForPath
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleDimens
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp

/**
 * Compact collapsible tool row. Port of the desktop's `ToolCalls::call_row`
 * (chat/toolcalls.rs): file-targeting calls show the file's type icon, edit and
 * write calls carry an inline diff per file with a +/- summary in the header,
 * and results render per tool kind (readFile as highlighted code with a line
 * gutter, subagent as markdown, everything else as monospace).
 */
@Composable
fun ToolCallRow(call: ToolCall, result: ToolResult?, cwd: String?) {
    var open by remember(call.id) { mutableStateOf(false) }
    val summary = toolCallSummary(call, cwd)
    val detail = result?.let { resultText(it).take(2000) }
    // Recomputed per call, not per frame: diffing a large write on every
    // recomposition would stall the transcript while a run streams. The diff
    // itself is the desktop's (Myers, 3-line context), shared over JNI.
    val diffs = remember(call.id, call.arguments) {
        if (call.isEdit || call.isWrite) SharedNative.callDiffs(call.name, call.arguments?.toString() ?: "{}") else emptyList()
    }
    val added = diffs.sumOf { it.diff.added }
    val removed = diffs.sumOf { it.diff.removed }
    val filePath = remember(call.id, call.arguments) { argumentPath(call) ?: diffs.firstOrNull()?.path }
    Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.FieldRadius)).background(NewTheme.Card)) {
        Row(modifier = Modifier.fillMaxWidth().clickable { open = !open }.padding(horizontal = 14.dp, vertical = 11.dp), verticalAlignment = Alignment.CenterVertically) {
            if (call.isFileTarget && filePath != null) {
                // File rows carry the real file-type icon, like desktop.
                FileIcon(filename = filePath, sizeDp = 16)
                Spacer(Modifier.width(8.dp))
            } else {
                Icon(getToolIcon(call), contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(8.dp))
            }
            Text(call.displayLabel, color = NewTheme.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            if (!summary.isNullOrEmpty()) {
                Text(summary, color = NewTheme.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
            } else {
                Spacer(modifier = Modifier.weight(1f))
            }
            if (diffs.isNotEmpty()) {
                DiffSummaryBadge(addedCount = added, removedCount = removed)
                Spacer(Modifier.width(6.dp))
            }
            if (result == null) {
                CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(15.dp))
            } else if (result.isError) {
                Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(15.dp))
            } else {
                Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(15.dp))
            }
            Spacer(Modifier.width(4.dp))
            Icon(if (open) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(15.dp))
        }
        if (open) {
            // Arguments and output sit on a darker inset than the row's card, like a
            // terminal pane under its title bar.
            Column(modifier = Modifier.fillMaxWidth().background(NewTheme.Output).padding(top = 4.dp, bottom = 12.dp)) {
                // A diff replaces the raw arguments — the diff already shows the
                // new content, which is what the arguments were for.
                if (diffs.isNotEmpty()) {
                    diffs.forEach { fileDiff ->
                        DiffView(diff = fileDiff.diff, filePath = fileDiff.path)
                    }
                } else {
                    ToolArguments(call.arguments)
                }
                if (!detail.isNullOrEmpty()) {
                    if (call.isRead) {
                        ReadFileResult(detail, filePath)
                    } else if (call.isSubagent) {
                        Text("Result", color = NewTheme.TextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
                        CustomMarkdown(content = detail, modifier = Modifier.padding(horizontal = 12.dp))
                    } else {
                        Text("Result", color = NewTheme.TextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
                        Box(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp)) {
                            Text(detail, color = NewTheme.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, lineHeight = 17.sp)
                        }
                    }
                } else if (result == null) {
                    Text("Running…", color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 12.dp))
                }
            }
        }
    }
}

@Composable
private fun ToolArguments(arguments: kotlinx.serialization.json.JsonElement?) {
    if (arguments == null) return
    val pretty = remember(arguments) { prettyJson(arguments) }
    Text("Arguments", color = NewTheme.TextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
    Box(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp)) {
        Text(pretty, color = NewTheme.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, lineHeight = 17.sp)
    }
}

/** readFile output as syntax-highlighted code with the tool's line numbers. */
@Composable
private fun ReadFileResult(raw: String, filePath: String?) {
    val parsed = remember(raw) { SharedNative.parseReadFileOutput(raw) }
        ?: return
    val numbers = remember(parsed.first) { parsed.first.split("\n") }
    val body = remember(parsed.second) { parsed.second.split("\n") }
    val language = remember(filePath) { languageForPath(filePath) }
    // The gutter is drawn here rather than by the viewer because a truncated
    // read carries the tool's own line numbers (which do not start at 1), and
    // the viewer's built-in gutter would renumber them from 1.
    val height = remember(body.size) { ((body.size * ConsoleDimens.CodeLineHeight + 16).coerceIn(80, 400)).dp }
    Text("Result", color = NewTheme.TextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
    Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp)) {
        // Line height has to equal the viewer's or the two grids drift apart.
        Text(
            numbers.joinToString("\n"),
            color = NewTheme.TextMuted,
            fontSize = ConsoleDimens.CodeGutterFontSizeSp.sp,
            fontFamily = ConsoleMonoFamily,
            lineHeight = ConsoleDimens.CodeLineHeight.sp,
            textAlign = TextAlign.End,
        )
        CodeViewer(
            code = body.joinToString("\n"),
            language = language,
            modifier = Modifier.fillMaxWidth().height(height),
            showLineNumbers = false,
            gutterColor = NewTheme.Output,
        )
    }
}

/** Pretty-print a tool call's arguments the way the desktop renders the
 * Arguments block (`serde_json::to_string_pretty`). */
private fun prettyJson(element: kotlinx.serialization.json.JsonElement): String =
    buildString { appendPretty(element, 0, this) }

private fun appendPretty(element: kotlinx.serialization.json.JsonElement, indent: Int, out: StringBuilder) {
    val pad = "  ".repeat(indent)
    val padInner = "  ".repeat(indent + 1)
    when (element) {
        is kotlinx.serialization.json.JsonObject -> {
            if (element.isEmpty()) { out.append("{}"); return }
            out.append("{\n")
            element.entries.forEachIndexed { index, (key, value) ->
                out.append(padInner).append("\"").append(key).append("\": ")
                appendPretty(value, indent + 1, out)
                if (index < element.size - 1) out.append(",")
                out.append("\n")
            }
            out.append(pad).append("}")
        }
        is kotlinx.serialization.json.JsonArray -> {
            if (element.isEmpty()) { out.append("[]"); return }
            out.append("[\n")
            element.forEachIndexed { index, value ->
                out.append(padInner)
                appendPretty(value, indent + 1, out)
                if (index < element.size - 1) out.append(",")
                out.append("\n")
            }
            out.append(pad).append("]")
        }
        is kotlinx.serialization.json.JsonPrimitive ->
            out.append(if (element.isString) "\"" + element.content.replace("\\", "\\\\").replace("\"", "\\\"") + "\"" else element.content)
    }
}
