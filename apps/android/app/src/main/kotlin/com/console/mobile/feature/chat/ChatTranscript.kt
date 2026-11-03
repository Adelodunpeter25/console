package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import console.v1.SessionFileChange

/**
 * The scrolling chat transcript: message bubbles with the run activity that
 * followed each user turn, plus the in-flight assistant footer.
 */
@Composable
internal fun ChatTranscript(
    sessionId: String,
    messages: List<AgentMessage>,
    listState: LazyListState,
    runs: List<RunActivityState>,
    userRunMap: Map<Int, Int>,
    assistantTurnChanges: Map<Int, List<SessionFileChange>>,
    latestUserIndex: Int,
    running: Boolean,
    isStreaming: Boolean,
    streamingText: String,
    streamingThinking: String,
    cwd: String?,
    onOpenChange: (SessionFileChange) -> Unit,
    onFork: (String) -> Unit,
) {
    LazyColumn(
        state = listState,
        contentPadding = PaddingValues(top = PageHeaderHeight + 8.dp, bottom = 120.dp),
        modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
    ) {
        itemsIndexed(messages, key = { _, m -> m.id ?: "${m.createdAt}-$sessionId" }) { index, msg ->
            MessageBubbleItem(
                item = msg,
                fileChanges = assistantTurnChanges[index].orEmpty(),
                onOpenChange = onOpenChange,
                onFork = onFork,
            )
            val runIdx = userRunMap[index]
            if (runIdx != null && runIdx < runs.size) {
                RunActivity(activity = runs[runIdx], running = running && index == latestUserIndex, cwd = cwd)
                RunVisuals(runs[runIdx])
            }
        }
        if (isStreaming && (streamingText.isNotEmpty() || streamingThinking.isNotEmpty())) {
            item(key = "streaming") {
                AssistantBubble(
                    textContent = streamingText.ifBlank { null },
                    thinkingContent = streamingThinking.ifBlank { null },
                    isStreaming = true,
                    createdAt = null,
                )
            }
        } else if (latestUserIndex == -1 && runs.isNotEmpty() && running) {
            item(key = "unattached-run") {
                RunActivity(activity = runs.last(), running = true, cwd = cwd)
                RunVisuals(runs.last())
            }
        }
    }
}
