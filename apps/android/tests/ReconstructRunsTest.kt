package com.console.mobile.core.chat

import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.ToolResultMessage
import com.console.mobile.data.model.UserMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ReconstructRunsTest {
    @Test fun splitsOnUserAndMatchesResults() {
        val msgs = listOf(
            UserMessage(content = "a", createdAt = 1000),
            AssistantMessage(content = listOf(ToolCallPart(ToolCall("c1", "bash"))), createdAt = 1500),
            ToolResultMessage(results = listOf(ToolResult("c1", "bash")), createdAt = 2000),
            UserMessage(content = "b", createdAt = 3000),
            AssistantMessage(content = listOf(TextPart("final")), createdAt = 3500),
        )
        val runs = reconstructRuns(msgs)
        assertEquals(2, runs.size)
        val first = runs[0]
        assertEquals(1000L, first.startedAt)
        assertEquals(1000L, first.elapsedMs)
        val tool = first.events.filterIsInstance<ActivityEvent.ToolCallEvent>().first()
        assertTrue(tool.result != null)
        // second run has no tool calls -> no events
        assertTrue(runs[1].events.isEmpty())
    }

    @Test fun emptyMessages() {
        assertTrue(reconstructRuns(emptyList()).isEmpty())
    }
}
