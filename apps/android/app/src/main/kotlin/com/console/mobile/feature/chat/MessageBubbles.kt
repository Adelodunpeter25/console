package com.console.mobile.feature.chat

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Warning
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import com.console.mobile.core.util.formatMessageTime
import com.console.mobile.core.util.getFileName
import com.console.mobile.core.util.getToolLabel
import com.console.mobile.core.util.resultText
import com.console.mobile.core.util.toolCallSummary
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
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily

/**
 * Port of components/chat/messages/message-bubbles.tsx.
 * UserBubble (right, elevated bg + attachments + copy), AssistantBubble
 * (thinking collapsible + markdown + streaming caret + Done row), ToolActivityRow.
 */
@Composable
fun UserBubble(content: String, createdAt: Long?, attachments: List<ImagePart> = emptyList()) {
    val context = LocalContext.current
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalAlignment = Alignment.End) {
        Column(
            modifier = Modifier.fillMaxWidth(0.85f).clip(RoundedCornerShape(20.dp, 20.dp, 20.dp, 6.dp))
                .background(ConsoleColors.SurfaceElevated).padding(horizontal = 16.dp, vertical = 10.dp),
        ) {
            if (attachments.isNotEmpty()) {
                Row(modifier = Modifier.padding(bottom = 6.dp)) {
                    attachments.forEach { att ->
                        val uri = "data:${att.mimeType};base64,${att.data}"
                        AsyncImage(model = uri, contentDescription = "Attachment", modifier = Modifier.size(80.dp).clip(RoundedCornerShape(12.dp)), contentScale = ContentScale.Crop)
                    }
                }
            }
            if (content.isNotEmpty()) {
                Text(content, color = ConsoleColors.TextPrimary, fontSize = 15.sp, lineHeight = 22.sp)
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 4.dp, end = 2.dp)) {
            Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = ConsoleColors.TextSecondary.copy(alpha = 0.7f), fontSize = 11.sp)
            if (content.isNotEmpty()) {
                CopyButton(text = content, context = context)
            }
        }
    }
}

@Composable
fun AssistantBubble(textContent: String?, thinkingContent: String?, isStreaming: Boolean, createdAt: Long?) {
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
            MarkdownText(content = textContent, streaming = isStreaming)
        }
        if (!isStreaming && !showTyping && textContent.isNullOrEmpty() && thinkingContent.isNullOrEmpty()) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(13.dp))
                Text("Done", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 6.dp))
            }
        }
        if (!isStreaming && (createdAt != null || !textContent.isNullOrEmpty() || !thinkingContent.isNullOrEmpty())) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 6.dp, start = 2.dp)) {
                Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = ConsoleColors.TextSecondary.copy(alpha = 0.7f), fontSize = 11.sp)
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
private fun CopyButton(text: String, context: Context) {
    var copied by remember(text) { mutableStateOf(false) }
    IconButton(onClick = {
        try {
            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            cm.setPrimaryClip(ClipData.newPlainText("console", text))
            copied = true
        } catch (_: Exception) {}
    }, modifier = Modifier.size(24.dp)) {
        if (copied) Icon(Icons.Filled.Check, contentDescription = "Copied", tint = Color(0xFF34D399), modifier = Modifier.size(12.dp))
        else Icon(Icons.Filled.ContentCopy, contentDescription = "Copy", tint = ConsoleColors.TextMuted, modifier = Modifier.size(12.dp))
    }
}

@Composable
private fun TypingDots() {
    Row(modifier = Modifier.padding(vertical = 4.dp)) {
        repeat(3) { i ->
            Box(modifier = Modifier.padding(end = 4.dp).size(6.dp).clip(androidx.compose.foundation.shape.CircleShape).background(ConsoleColors.TextSecondary))
        }
    }
}

@Composable
fun ThinkingBlock(text: String, isStreaming: Boolean) {
    var expanded by remember { mutableStateOf(false) }
    Column(modifier = Modifier.padding(bottom = 8.dp)) {
        Row(
            modifier = Modifier.clip(RoundedCornerShape(8.dp)).background(Color.White.copy(alpha = 0.06f)).clickable { expanded = !expanded }.padding(horizontal = 8.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Filled.AutoAwesome, contentDescription = null, tint = Color(0xFFFB923C), modifier = Modifier.size(13.dp))
            Text(if (isStreaming) "Thinking…" else "Thought", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 6.dp))
            Icon(if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(12.dp))
        }
        if (expanded) {
            Box(modifier = Modifier.fillMaxWidth().padding(top = 6.dp).clip(RoundedCornerShape(12.dp)).background(Color.White.copy(alpha = 0.03f)).padding(horizontal = 12.dp, vertical = 10.dp)) {
                Text(text, color = ConsoleColors.TextSecondary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily, lineHeight = 20.sp)
            }
        }
    }
}

/** Compact collapsible tool-activity row (running / done / failed). */
@Composable
fun ToolActivityRow(name: String, isRunning: Boolean, isError: Boolean, detail: String?) {
    var expanded by remember { mutableStateOf(false) }
    val statusColor = if (isError) Color(0xFFF87171) else if (isRunning) Color(0xFFFB923C) else Color(0xFF34D399)
    val statusBg = statusColor.copy(alpha = 0.1f)
    val shape = RoundedCornerShape(12.dp)
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp).clip(shape).background(Color.White.copy(alpha = 0.04f)).clickable { expanded = !expanded }) {
        Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(modifier = Modifier.size(28.dp).clip(RoundedCornerShape(8.dp)).background(statusBg), contentAlignment = Alignment.Center) {
                if (isRunning) CircularProgressIndicator(color = statusColor, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                else if (isError) Icon(Icons.Filled.Warning, contentDescription = null, tint = statusColor, modifier = Modifier.size(14.dp))
                else Icon(Icons.Filled.Check, contentDescription = null, tint = statusColor, modifier = Modifier.size(13.dp))
            }
            Text(getToolLabel(name), color = ConsoleColors.TextPrimary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 10.dp))
            if (isRunning) {
                Text(detail ?: "Running", color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
            } else {
                Text(if (isError) "FAILED" else "DONE", color = statusColor, fontSize = 10.sp, fontWeight = FontWeight.Bold, fontFamily = ConsoleMonoFamily)
                Icon(if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(12.dp))
            }
        }
        if (expanded && !detail.isNullOrEmpty()) {
            Box(modifier = Modifier.fillMaxWidth().background(Color.White.copy(alpha = 0.02f)).padding(horizontal = 14.dp, vertical = 10.dp)) {
                Text(detail, color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, lineHeight = 17.sp)
            }
        }
    }
}

@Composable
fun ToolCallRow(call: ToolCall, result: ToolResult?, cwd: String?) {
    var open by remember(call.id) { mutableStateOf(false) }
    val summary = toolCallSummary(call, cwd)
    val detail = result?.let { resultText(it).take(2000) }
    val shape = RoundedCornerShape(10.dp)
    Column(modifier = Modifier.fillMaxWidth().clip(shape).background(Color.White.copy(alpha = 0.02f)).border(1.dp, Color.White.copy(alpha = 0.06f), shape)) {
        Row(modifier = Modifier.fillMaxWidth().clickable { open = !open }.padding(horizontal = 12.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(getToolLabel(call.name), color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold)
            if (!summary.isNullOrEmpty()) {
                Text(summary, color = ConsoleColors.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
            } else {
                androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
            }
            if (result == null) {
                CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(13.dp))
            } else if (result.isError) {
                Icon(Icons.Filled.Warning, contentDescription = null, tint = Color(0xFFF87171), modifier = Modifier.size(13.dp))
            } else {
                Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(13.dp))
            }
            Icon(if (open) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(13.dp))
        }
        if (open) {
            Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 10.dp)) {
                if (!detail.isNullOrEmpty()) {
                    Box(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
                        Text(detail, color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, lineHeight = 17.sp)
                    }
                } else if (result == null) {
                    Text("Running…", color = ConsoleColors.TextMuted, fontSize = 12.sp)
                }
            }
        }
    }
}

/** Renders one agent message: user / assistant / toolResult(suppressed). */
@Composable
fun MessageBubbleItem(item: AgentMessage) {
    when (item) {
        is UserMessage -> UserBubble(content = item.content, createdAt = item.createdAt, attachments = item.attachments)
        is ToolResultMessage -> {}
        is AssistantMessage -> {
            val hasToolCalls = item.content.any { it is ToolCallPart }
            if (hasToolCalls) return
            val text = item.content.filterIsInstance<TextPart>().joinToString("\n\n") { it.text }
            val thinking = item.content.filterIsInstance<ThinkingPart>().joinToString("\n\n") { it.text }
            if (text.isBlank() && thinking.isBlank()) return
            AssistantBubble(textContent = text.ifBlank { null }, thinkingContent = thinking.ifBlank { null }, isStreaming = false, createdAt = item.createdAt)
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
