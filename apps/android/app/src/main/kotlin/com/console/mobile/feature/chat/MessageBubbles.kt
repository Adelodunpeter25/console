package com.console.mobile.feature.chat
import com.console.mobile.feature.chat.markdown.CustomMarkdown

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import console.v1.SessionFileChange
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Copy
import io.github.lyxnx.compose.ui.tablericons.outline.Sparkles
import coil3.compose.AsyncImage
import com.console.mobile.core.util.argumentPath
import com.console.mobile.core.util.extractWriteArgs
import com.console.mobile.core.util.fileCallDiffs
import com.console.mobile.core.util.formatMessageTime
import com.console.mobile.core.util.getFileName
import com.console.mobile.core.util.getToolIcon
import com.console.mobile.core.util.getToolLabel
import com.console.mobile.core.util.isEditFileTool
import com.console.mobile.core.util.isFileTargetTool
import com.console.mobile.core.util.isReadFileTool
import com.console.mobile.core.util.isSubagentTool
import com.console.mobile.core.util.isWriteFileTool
import com.console.mobile.core.util.parseFileMentions
import com.console.mobile.core.util.parseReadFileOutput
import com.console.mobile.core.util.resultText
import com.console.mobile.core.util.toolCallSummary
import com.console.mobile.ui.code.CodeViewer
import com.console.mobile.ui.code.languageForPath
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.ImagePart
import com.console.mobile.data.model.MessageContent
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.ToolResultMessage
import com.console.mobile.data.model.UserMessage
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.components.attachmentBytes
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleDimens
import com.console.mobile.ui.theme.ConsoleMonoFamily

/**
 * Port of components/chat/messages/message-bubbles.tsx.
 * UserBubble (right, elevated bg + attachments + copy), AssistantBubble
 * (thinking collapsible + markdown + streaming caret + Done row), ToolCallRow.
 */
@Composable
fun UserBubble(content: String, createdAt: Long?, attachments: List<ImagePart> = emptyList()) {
    val context = LocalContext.current
    var preview by remember { mutableStateOf<ImagePart?>(null) }
    var expanded by remember(content) { mutableStateOf(false) }
    var hasOverflow by remember(content) { mutableStateOf(false) }
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalAlignment = Alignment.End) {
        androidx.compose.foundation.layout.BoxWithConstraints(modifier = Modifier.align(Alignment.End)) {
            Column(
                modifier = Modifier.widthIn(min = 64.dp, max = maxWidth * 0.85f).clip(RoundedCornerShape(20.dp))
                    .background(NewTheme.UserBubble).padding(horizontal = 16.dp, vertical = 12.dp),
            ) {
                if (attachments.isNotEmpty()) {
                    Row(modifier = Modifier.padding(bottom = 6.dp)) {
                        attachments.forEach { att ->
                            val bytes = remember(att) { attachmentBytes(att.data) }
                            if (bytes != null) {
                                AsyncImage(model = bytes, contentDescription = "Attachment", modifier = Modifier.size(80.dp).clip(RoundedCornerShape(12.dp)).clickable { preview = att }, contentScale = ContentScale.Crop)
                            }
                        }
                    }
                }
                if (content.isNotEmpty()) {
                    val mentions = remember(content) { parseFileMentions(content) }
                    val maxLines = if (expanded) Int.MAX_VALUE else 4
                    if (mentions.isEmpty()) {
                        Text(
                            content,
                            color = NewTheme.OnUserBubble,
                            fontSize = 16.sp,
                            lineHeight = 23.sp,
                            maxLines = maxLines,
                            overflow = TextOverflow.Ellipsis,
                            onTextLayout = { textLayoutResult ->
                                if (!expanded) {
                                    hasOverflow = textLayoutResult.hasVisualOverflow || textLayoutResult.lineCount > 4
                                }
                            },
                        )
                    } else {
                        val (annotated, inlineContent) = mentionAnnotatedString(content, mentions)
                        Text(
                            annotated,
                            inlineContent = inlineContent,
                            color = NewTheme.OnUserBubble,
                            fontSize = 16.sp,
                            lineHeight = 23.sp,
                            maxLines = maxLines,
                            overflow = TextOverflow.Ellipsis,
                            onTextLayout = { textLayoutResult ->
                                if (!expanded) {
                                    hasOverflow = textLayoutResult.hasVisualOverflow || textLayoutResult.lineCount > 4
                                }
                            },
                        )
                    }
                    if (hasOverflow) {
                        Text(
                            text = if (expanded) "Show less" else "Show more",
                            color = NewTheme.Accent,
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier
                                .padding(top = 6.dp)
                                .clickable { expanded = !expanded },
                        )
                    }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 4.dp, end = 2.dp)) {
            Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = NewTheme.TextMuted, fontSize = 12.sp)
            if (content.isNotEmpty()) {
                CopyButton(text = content, context = context)
            }
        }
    }
    val current = preview
    if (current != null && attachments.contains(current)) {
        val bytes = remember(current) { attachmentBytes(current.data) }
        if (bytes != null) {
            ImagePreviewDialog(image = bytes, onDismiss = { preview = null })
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun AssistantBubble(
    textContent: String?,
    thinkingContent: String?,
    isStreaming: Boolean,
    createdAt: Long?,
    fileChanges: List<SessionFileChange> = emptyList(),
    onOpenChange: ((SessionFileChange) -> Unit)? = null,
) {
    val context = LocalContext.current
    val hasContent = !textContent.isNullOrEmpty() || !thinkingContent.isNullOrEmpty()
    val showTyping = isStreaming && !hasContent
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp)) {
        if (showTyping) {
            TypingDots()
        }
        if (!thinkingContent.isNullOrEmpty()) {
            ThinkingBlock(text = thinkingContent, isStreaming = isStreaming)
        }
        if (!textContent.isNullOrEmpty()) {
            CustomMarkdown(content = textContent, streaming = isStreaming)
        }
        if (!isStreaming && !showTyping && textContent.isNullOrEmpty() && thinkingContent.isNullOrEmpty()) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(14.dp))
                Text("Done", color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(start = 6.dp))
            }
        }
        if (!isStreaming && fileChanges.isNotEmpty()) {
            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 2.dp),
            ) {
                fileChanges.forEach { change ->
                    TurnChangeChip(
                        change = change,
                        onClick = { onOpenChange?.invoke(change) },
                    )
                }
            }
        }
        if (!isStreaming && (createdAt != null || !textContent.isNullOrEmpty() || !thinkingContent.isNullOrEmpty() || fileChanges.isNotEmpty())) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 6.dp, start = 2.dp)) {
                Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = NewTheme.TextMuted, fontSize = 12.sp)
                val copyable = buildList {
                    if (!thinkingContent.isNullOrEmpty()) add("Thought:\n$thinkingContent")
                    if (!textContent.isNullOrEmpty()) add(textContent)
                }.joinToString("\n\n")
                if (copyable.isNotEmpty()) CopyButton(text = copyable, context = context)
            }
        }
    }
}

@Composable
fun TurnChangeChip(
    change: SessionFileChange,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val name = remember(change.path) { getFileName(change.path) }
    val isDeleted = change.status == "deleted"
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(NewTheme.Card)
            .border(1.dp, NewTheme.Divider, RoundedCornerShape(6.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FileIcon(filename = name, sizeDp = 13, modifier = Modifier.padding(end = 6.dp))
        Text(
            text = name,
            color = if (isDeleted) NewTheme.TextMuted else NewTheme.TextPrimary,
            fontSize = 12.sp,
            fontFamily = ConsoleMonoFamily,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textDecoration = if (isDeleted) TextDecoration.LineThrough else null,
        )
        if (change.additions > 0 || change.deletions > 0) {
            Spacer(modifier = Modifier.size(6.dp))
            DiffSummaryBadge(addedCount = change.additions, removedCount = change.deletions)
        }
    }
}

@Composable
private fun CopyButton(text: String, context: Context) {
    var copied by remember(text) { mutableStateOf(false) }
    IconButton(onClick = {
        try {
            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            cm.setPrimaryClip(ClipData.newPlainText("console", text))
            copied = true
        } catch (_: Exception) {}
    }, modifier = Modifier.size(24.dp)) {
        if (copied) Icon(TablerIcons.Outline.Check, contentDescription = "Copied", tint = NewTheme.Success, modifier = Modifier.size(13.dp))
        else Icon(TablerIcons.Outline.Copy, contentDescription = "Copy", tint = NewTheme.TextMuted, modifier = Modifier.size(13.dp))
    }
}

@Composable
private fun TypingDots() {
    Row(modifier = Modifier.padding(vertical = 4.dp)) {
        repeat(3) { i ->
            Box(modifier = Modifier.padding(end = 4.dp).size(6.dp).clip(androidx.compose.foundation.shape.CircleShape).background(NewTheme.TextSecondary))
        }
    }
}

@Composable
fun ThinkingBlock(text: String, isStreaming: Boolean) {
    var expanded by remember { mutableStateOf(false) }
    Column(modifier = Modifier.padding(bottom = 8.dp)) {
        Row(
            modifier = Modifier.clip(RoundedCornerShape(NewTheme.ChipRadius)).background(NewTheme.Card).clickable { expanded = !expanded }.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(TablerIcons.Outline.Sparkles, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(14.dp))
            Text(if (isStreaming) "Thinking…" else "Thought", color = NewTheme.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 6.dp))
            Icon(if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(12.dp))
        }
        if (expanded) {
            Box(modifier = Modifier.fillMaxWidth().padding(top = 6.dp).clip(RoundedCornerShape(NewTheme.FieldRadius)).background(NewTheme.Card).padding(horizontal = 14.dp, vertical = 12.dp)) {
                Text(text, color = NewTheme.TextSecondary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily, lineHeight = 20.sp)
            }
        }
    }
}

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
    val isFileTool = isEditFileTool(call.name) || isWriteFileTool(call.name)
    // Recomputed per call, not per frame: diffing a large write on every
    // recomposition would stall the transcript while a run streams.
    val diffs = remember(call.id, call.arguments) { if (isFileTool) fileCallDiffs(call) else emptyList() }
    val added = diffs.sumOf { it.diff.addedCount }
    val removed = diffs.sumOf { it.diff.removedCount }
    val filePath = remember(call.id, call.arguments) { argumentPath(call) ?: extractWriteArgs(call)?.first }
    Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.FieldRadius)).background(NewTheme.Card)) {
        Row(modifier = Modifier.fillMaxWidth().clickable { open = !open }.padding(horizontal = 14.dp, vertical = 11.dp), verticalAlignment = Alignment.CenterVertically) {
            if (isFileTargetTool(call.name) && filePath != null) {
                // File rows carry the real file-type icon, like desktop.
                FileIcon(filename = filePath, sizeDp = 13, modifier = Modifier.padding(end = 6.dp))
            } else {
                Icon(getToolIcon(call.name), contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(14.dp).padding(end = 6.dp))
            }
            Text(getToolLabel(call.name), color = NewTheme.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            if (!summary.isNullOrEmpty()) {
                Text(summary, color = NewTheme.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
            } else {
                Spacer(modifier = Modifier.weight(1f))
            }
            if (diffs.isNotEmpty()) {
                DiffSummaryBadge(addedCount = added, removedCount = removed)
            }
            if (result == null) {
                CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(13.dp))
            } else if (result.isError) {
                Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(14.dp))
            } else {
                Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(14.dp))
            }
            Icon(if (open) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(13.dp))
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
                    if (isReadFileTool(call.name)) {
                        ReadFileResult(detail, filePath)
                    } else if (isSubagentTool(call.name)) {
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
    val (numbers, body) = remember(raw) { parseReadFileOutput(raw) }
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

/** Renders one agent message: user / assistant / toolResult(suppressed). */
@Composable
fun MessageBubbleItem(
    item: AgentMessage,
    fileChanges: List<SessionFileChange> = emptyList(),
    onOpenChange: ((SessionFileChange) -> Unit)? = null,
) {
    when (item) {
        is UserMessage -> UserBubble(content = item.content, createdAt = item.createdAt, attachments = item.attachments)
        is ToolResultMessage -> {}
        is AssistantMessage -> {
            val hasToolCalls = item.content.any { it is ToolCallPart }
            if (hasToolCalls) return
            val text = item.content.filterIsInstance<TextPart>().joinToString("\n\n") { it.text }
            val thinking = item.content.filterIsInstance<ThinkingPart>().joinToString("\n\n") { it.text }
            if (text.isBlank() && thinking.isBlank() && fileChanges.isEmpty()) return
            AssistantBubble(
                textContent = text.ifBlank { null },
                thinkingContent = thinking.ifBlank { null },
                isStreaming = false,
                createdAt = item.createdAt,
                fileChanges = fileChanges,
                onOpenChange = onOpenChange,
            )
        }
    }
}

fun visibleMessages(messages: List<AgentMessage>): List<AgentMessage> =
    messages.filter { msg ->
        when (msg) {
            is ToolResultMessage -> false
            is AssistantMessage -> msg.content.none { it is ToolCallPart }
            else -> true
        }
    }

fun messageTextOf(content: List<MessageContent>): String =
    content.filterIsInstance<TextPart>().joinToString("\n\n") { it.text }

fun messageThinkingOf(content: List<MessageContent>): String =
    content.filterIsInstance<ThinkingPart>().joinToString("\n\n") { it.text }

fun messageFileName(path: String?): String = getFileName(path)
