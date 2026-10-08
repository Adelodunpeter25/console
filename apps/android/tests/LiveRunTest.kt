package com.console.mobile

import com.console.mobile.core.chat.ActivityEvent
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.withLiveRun
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.UserMessage
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * A run started on another device is attached to after its prompt is already
 * in history. The live run must pair with that prompt (one run per user
 * message), not trail after it where nothing renders it.
 */
class LiveRunTest {
    private val history = listOf(
        UserMessage(id = "u1", createdAt = 1, content = "first"),
        AssistantMessage(id = "a1", createdAt = 2, content = listOf(TextPart("done"))),
        UserMessage(id = "u2", createdAt = 3, content = "proceed to fix"),
        AssistantMessage(
            id = "a2", createdAt = 4,
            content = listOf(ToolCallPart(ToolCall(id = "t1", name = "read_file"))),
        ),
    )

    @Test
    fun reopensLatestPromptRunInsteadOfAppending() {
        val runs = withLiveRun(history, emptyList(), nowMs = 10)
        assertEquals(2, runs.size)
        assertEquals(RunStatus.Completed, runs[0].status)
        assertEquals(RunStatus.Working, runs[1].status)
        assertEquals(listOf("t1"), runs[1].events.map { it.id })
    }

    @Test
    fun keepsLiveEventsWhenHistoryArrivesAfterAttach() {
        val live = RunActivityState(
            runId = "live", startedAt = 5, status = RunStatus.Working,
            events = listOf(ActivityEvent.ToolCallEvent("t2", ToolCall(id = "t2", name = "grep"))),
        )
        val runs = withLiveRun(history, listOf(live), nowMs = 10)
        assertEquals(2, runs.size)
        assertEquals("live", runs[1].runId)
        assertEquals(listOf("t1", "t2"), runs[1].events.map { it.id })
    }

    @Test
    fun opensRunWhenNoHistoryYet() {
        val runs = withLiveRun(emptyList(), emptyList(), nowMs = 10)
        assertEquals(1, runs.size)
        assertEquals(RunStatus.Working, runs[0].status)
    }
}
