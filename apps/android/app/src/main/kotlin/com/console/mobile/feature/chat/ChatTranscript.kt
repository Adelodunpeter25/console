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
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.theme.NewTheme
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
    userRunMap: Map<Int, List<Int>>,
    assistantTurnChanges: Map<Int, List<SessionFileChange>>,
    latestUserIndex: Int,
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
    onOpenHtmlExternal: (String) -> Unit = {},
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
                onReply = onReply,
            )
            val runIndices = userRunMap[index].orEmpty()
            for (runIdx in runIndices) {
                if (runIdx < runs.size) {
                    val run = runs[runIdx]
                    run.dividerLabel?.let { label ->
                        Text(
                            text = label,
                            color = NewTheme.TextSecondary,
                            fontSize = 12.sp,
                            modifier = Modifier.padding(start = 4.dp, top = 8.dp, bottom = 2.dp),
                        )
                    }
                    RunActivity(activity = run, running = running && index >= latestUserIndex, cwd = cwd)
                    RunVisuals(
                        activity = run,
                        onOpenHtmlPreview = onOpenHtmlPreview,
                        onOpenHtmlEditor = onOpenHtmlEditor,
                        onOpenHtmlExternal = onOpenHtmlExternal,
                    )
                }
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
        } else if (latestUserIndex == -1 && runs.isNotEmpty() && running) {
            item(key = "unattached-run") {
                RunActivity(activity = runs.last(), running = true, cwd = cwd)
                RunVisuals(
                    activity = runs.last(),
                    onOpenHtmlPreview = onOpenHtmlPreview,
                    onOpenHtmlEditor = onOpenHtmlEditor,
                    onOpenHtmlExternal = onOpenHtmlExternal,
                )
            }
        }
    }
}
