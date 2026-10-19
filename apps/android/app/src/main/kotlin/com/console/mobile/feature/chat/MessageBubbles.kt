package com.console.mobile.feature.chat

import androidx.compose.runtime.Composable
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResultMessage
import com.console.mobile.data.model.UserMessage
import console.v1.SessionFileChange

/** Entry point for one transcript row: dispatch to the matching bubble. */
@Composable
fun MessageBubbleItem(
    item: AgentMessage,
    fileChanges: List<SessionFileChange> = emptyList(),
    onOpenChange: ((SessionFileChange) -> Unit)? = null,
    onFork: ((String) -> Unit)? = null,
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
                messageId = item.id,
                onFork = onFork,
                tokPerSec = item.tokPerSec,
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
