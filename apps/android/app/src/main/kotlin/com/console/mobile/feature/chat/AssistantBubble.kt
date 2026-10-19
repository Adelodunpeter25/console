package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.util.formatMessageTime
import com.console.mobile.core.util.formatTokensPerSecond
import com.console.mobile.feature.chat.markdown.CustomMarkdown
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowBearRight2
import io.github.lyxnx.compose.ui.tablericons.outline.Check

/** Assistant turn: thinking, markdown, change chips, then the metadata footer. */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun AssistantBubble(
    textContent: String?,
    thinkingContent: String?,
    isStreaming: Boolean,
    createdAt: Long?,
    fileChanges: List<SessionFileChange> = emptyList(),
    onOpenChange: ((SessionFileChange) -> Unit)? = null,
    messageId: String? = null,
    onFork: ((String) -> Unit)? = null,
    tokPerSec: Double? = null,
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
            var expandedChanges by remember(fileChanges) { mutableStateOf(false) }
            val foldable = fileChanges.size > 4
            val visibleChanges = if (foldable && !expandedChanges) fileChanges.take(3) else fileChanges

            FlowRow(
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp, bottom = 2.dp),
            ) {
                visibleChanges.forEach { change ->
                    TurnChangeChip(
                        change = change,
                        onClick = { onOpenChange?.invoke(change) },
                    )
                }
                if (foldable) {
                    MoreChangesChip(
                        expanded = expandedChanges,
                        remainingCount = fileChanges.size - 3,
                        onClick = { expandedChanges = !expandedChanges },
                    )
                }
            }
        }
        if (!isStreaming && (createdAt != null || !textContent.isNullOrEmpty() || !thinkingContent.isNullOrEmpty() || fileChanges.isNotEmpty())) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 6.dp, start = 2.dp)) {
                Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = NewTheme.TextMuted, fontSize = 12.sp)
                formatTokensPerSecond(tokPerSec)?.let { rate ->
                    Text(rate, color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(start = 8.dp))
                }
                if (onFork != null && messageId != null) ForkButton(onClick = { onFork.invoke(messageId) })
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
private fun ForkButton(onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.size(24.dp)) {
        Icon(TablerIcons.Outline.ArrowBearRight2, contentDescription = "Fork chat from here", tint = NewTheme.TextMuted, modifier = Modifier.size(13.dp))
    }
}

@Composable
private fun TypingDots() {
    Row(modifier = Modifier.padding(vertical = 4.dp)) {
        repeat(3) {
            Box(modifier = Modifier.padding(end = 4.dp).size(6.dp).clip(CircleShape).background(NewTheme.TextSecondary))
        }
    }
}
