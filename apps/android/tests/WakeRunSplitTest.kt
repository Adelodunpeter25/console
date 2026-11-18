package com.console.mobile

import com.console.mobile.core.chat.ActivityEvent
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.WakeRecord
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.buildUserRunMap
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.ToolResultMessage
import com.console.mobile.data.model.UserMessage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A server wake turn (backgroundWorkContinue) must split run activity under
 * the same user message instead of merging into the previous turn — mirrors
 * the desktop RunBoundary work.
 */
class WakeRunSplitTest {
    private fun toolCall(id: String) = ToolCallPart(ToolCall(id = id, name = "bash"))

    private fun history() = listOf(
        UserMessage(id = "u1", createdAt = 1_000, content = "run it"),
        AssistantMessage(id = "a1", createdAt = 2_000, content = listOf(toolCall("c1"))),
        ToolResultMessage(id = "r1", createdAt = 3_000, results = listOf(
            ToolResult(toolCallId = "c1", toolName = "bash", content = null, isError = false),
        )),
        AssistantMessage(id = "a2", createdAt = 10_000, content = listOf(toolCall("c2"))),
        ToolResultMessage(id = "r2", createdAt = 11_000, results = listOf(
            ToolResult(toolCallId = "c2", toolName = "bash", content = null, isError = false),
        )),
    )

    @Test
    fun reconstructSplitsAtWakeRecord() {
        val records = listOf(WakeRecord(message = "Bash tool completed, continuing agent", at = 9_000))
        val runs = reconstructRuns(history(), records)
        assertEquals(2, runs.size)
        assertNull(runs[0].dividerLabel)
        assertEquals("Bash tool completed, continuing agent", runs[1].dividerLabel)
        assertEquals(10_000L, runs[1].startedAt)
        assertEquals(
            listOf("c1"),
            runs[0].events.filterIsInstance<ActivityEvent.ToolCallEvent>().map { it.call.id },
        )
        assertEquals(
            listOf("c2"),
            runs[1].events.filterIsInstance<ActivityEvent.ToolCallEvent>().map { it.call.id },
        )
    }

    @Test
    fun reconstructWithoutRecordsMergesAsBefore() {
        val runs = reconstructRuns(history())
        assertEquals(1, runs.size)
        assertNull(runs[0].dividerLabel)
    }

    @Test
    fun continueEventClosesWorkingRunAndOpensWakeRun() {
        val working = createChatSessionState().copy(
            runs = listOf(
                RunActivityState(
                    runId = "r1", startedAt = 1_000, status = RunStatus.Working,
                ),
            ),
        )
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"backgroundWorkContinue","message":"Bash tool completed, continuing agent","work":[{"id":"w1","sessionId":"s1","kind":"shell","status":"completed","createdAt":"2026-10-13T09:45:44Z","metadata":{"job_id":"j1","command":"npm run dev"}}]}""",
        )
        val after = applyChatEvent(working, event)
        assertEquals(2, after.runs.size)
        assertEquals(RunStatus.Completed, after.runs[0].status)
        assertEquals(RunStatus.Working, after.runs[1].status)
        assertEquals("Bash tool completed, continuing agent", after.runs[1].dividerLabel)
        assertEquals(1, after.wakeRecords.size)
        assertEquals("Bash tool completed, continuing agent", after.wakeRecords[0].message)
    }

    @Test
    fun continueEventOnIdleSessionStillRecordsWake() {
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"backgroundWorkContinue","message":"Subagent completed, continuing agent","work":[]}""",
        )
        val after = applyChatEvent(createChatSessionState(), event)
        assertEquals(1, after.wakeRecords.size)
        assertEquals("Subagent completed, continuing agent", after.wakeRecords[0].message)
        assertEquals(1, after.runs.size)
        assertEquals(RunStatus.Working, after.runs[0].status)
        assertEquals("Subagent completed, continuing agent", after.runs[0].dividerLabel)
    }

    @Test
    fun wakeRecordsAreCapped() {
        var session = createChatSessionState()
        repeat(12) {
            session = applyChatEvent(
                session,
                ConsoleJson.decodeFromString<AgentSessionEvent>(
                    """{"type":"backgroundWorkContinue","message":"wake $it","work":[]}""",
                ),
            )
        }
        assertEquals(8, session.wakeRecords.size)
        assertEquals("wake 11", session.wakeRecords.last().message)
    }

    @Test
    fun textOnlyTurnProducesNoToolEvents() {
        // reconstructRuns emits a run per user message; the UI hides it when
        // it has no tool calls (RunActivity early-returns), matching desktop's
        // "no block without tool calls".
        val messages = listOf(
            UserMessage(id = "u1", createdAt = 1_000, content = "hi"),
            AssistantMessage(
                id = "a1", createdAt = 2_000,
                content = listOf(TextPart("done")),
            ),
        )
        val runs = reconstructRuns(messages)
        assertEquals(1, runs.size)
        assertTrue(runs[0].events.none { it is ActivityEvent.ToolCallEvent })
    }

    @Test
    fun wakeRunAnchorsAfterFirstTurnReply() {
        val displayMessages = listOf(
            UserMessage(id = "u1", createdAt = 1_000, content = "fix it"),
            AssistantMessage(id = "a1", createdAt = 5_000, content = listOf(TextPart("waiting for check"))),
            AssistantMessage(id = "a2", createdAt = 15_000, content = listOf(TextPart("all done"))),
        )
        val runs = listOf(
            RunActivityState(runId = "r1", startedAt = 1_000, status = RunStatus.Completed),
            RunActivityState(
                runId = "r2",
                startedAt = 10_000,
                status = RunStatus.Completed,
                dividerLabel = "Background work completed, continuing agent",
            ),
        )
        val map = buildUserRunMap(displayMessages, runs)
        assertEquals(listOf(0), map[0])
        assertEquals(listOf(1), map[1])
        assertNull(map[2])
    }
}
