package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.ActivityEvent
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.TranscriptTurn
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange

/**
 * The scrolling chat transcript: server-assembled turns (prompt or wake
 * divider, run activity, and final assistant replies), plus the in-flight
 * assistant footer.
 */
@Composable
internal fun ChatTranscript(
    sessionId: String,
    turns: List<TranscriptTurn>,
    listState: LazyListState,
    turnChanges: Map<Int, List<SessionFileChange>>,
    running: Boolean,
    isStreaming: Boolean,
    streamingText: String,
    streamingThinking: String,
    cwd: String?,
    onOpenChange: (SessionFileChange) -> Unit,
    onFork: (String) -> Unit,
    onReply: ((String) -> Unit)? = null,
    onOpenHtmlPreview: (com.console.mobile.data.model.HtmlArtifact) -> Unit = {},
    onOpenHtmlEditor: (String) -> Unit = {},
    onExpandToolCall: (messageId: String, callId: String) -> Unit = { _, _ -> },
) {
    LazyColumn(
        state = listState,
        contentPadding = PaddingValues(top = PageHeaderHeight + 8.dp, bottom = 120.dp),
        modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
    ) {
        itemsIndexed(turns, key = { _, t -> "${t.id}-$sessionId" }) { index, turn ->
            val isLastTurn = index == turns.lastIndex
            turn.userMessage?.let { userMsg ->
                MessageBubbleItem(
                    item = userMsg,
                    fileChanges = emptyList(),
                    onOpenChange = onOpenChange,
                    onFork = onFork,
                    onReply = onReply,
                )
            } ?: turn.wakeLabel?.let { label ->
                Text(
                    text = label,
                    color = NewTheme.TextSecondary,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(start = 4.dp, top = 8.dp, bottom = 2.dp),
                )
            }

            val run = turn.run
            val hasToolCalls = run?.events?.any { it is ActivityEvent.ToolCallEvent } == true
            if (run != null && hasToolCalls) {
                val runWorking = isLastTurn && (running || run.status == RunStatus.Working)
                RunActivity(
                    activity = run,
                    running = runWorking,
                    cwd = cwd,
                    onExpandToolCall = onExpandToolCall,
                )
                RunVisuals(
                    activity = run,
                    onOpenHtmlPreview = onOpenHtmlPreview,
                    onOpenHtmlEditor = onOpenHtmlEditor,
                )
            }

            val changesForTurn = turnChanges[index].orEmpty()
            turn.responses.forEachIndexed { respIdx, resp ->
                val isLastResp = respIdx == turn.responses.lastIndex
                MessageBubbleItem(
                    item = resp,
                    fileChanges = if (isLastResp) changesForTurn else emptyList(),
                    onOpenChange = onOpenChange,
                    onFork = onFork,
                    onReply = onReply,
                )
            }
        }
        if (isStreaming && (streamingText.isNotEmpty() || streamingThinking.isNotEmpty())) {
            item(key = "streaming") {
                AssistantBubble(
                    textContent = streamingText.ifBlank { null },
                    thinkingContent = streamingThinking.ifBlank { null },
                    isStreaming = true,
                    createdAt = null,
                    onReply = onReply,
                )
            }
        }
    }
}
