package com.console.mobile

import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.TranscriptTurn
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import org.junit.Assert.assertTrue
import org.junit.Test

class ServerRunElapsedTest {
    @Test
    fun sessionStartAnchorsWorkingRunToServerElapsed() {
        val localStart = System.currentTimeMillis()
        val session = createChatSessionState().copy(
            turns = listOf(
                TranscriptTurn(
                    id = "turn-1",
                    run = RunActivityState(
                        runId = "run-1",
                        startedAt = localStart,
                        status = RunStatus.Working,
                    ),
                ),
            ),
        )
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"sessionStart","startedAt":"2026-10-13T09:45:44Z","elapsedMs":4500}""",
        )

        val before = System.currentTimeMillis()
        val updated = applyChatEvent(session, event)
        val after = System.currentTimeMillis()
        val startedAt = updated.turns.single().run!!.startedAt!!

        assertTrue(startedAt >= before - 4500)
        assertTrue(startedAt <= after - 4500)
        assertTrue(updated.running)
    }
}
