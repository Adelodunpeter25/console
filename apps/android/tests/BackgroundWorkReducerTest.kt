package com.console.mobile

import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.runningBackgroundShells
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.BackgroundWorkOutput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class BackgroundWorkReducerTest {
    @Test
    fun upsertsRunningShellThenDropsItWhenCompleted() {
        val started = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"backgroundWorkStarted","work":{"id":"work_test","sessionId":"session-1","kind":"shell","status":"running","createdAt":"2026-10-13T09:45:44Z","metadata":{"job_id":"job_test","command":"npm run dev"}}}""",
        )
        val afterStart = applyChatEvent(createChatSessionState(), started)
        assertEquals(listOf("work_test"), runningBackgroundShells(afterStart.backgroundWork).map { it.id })

        val completed = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"backgroundWorkCompleted","work":{"id":"work_test","sessionId":"session-1","kind":"shell","status":"completed","createdAt":"2026-10-13T09:45:44Z","metadata":{"job_id":"job_test","command":"npm run dev"}}}""",
        )
        val afterDone = applyChatEvent(afterStart, completed)
        assertEquals(1, afterDone.backgroundWork.size)
        assertEquals("completed", afterDone.backgroundWork.single().status)
        assertTrue(runningBackgroundShells(afterDone.backgroundWork).isEmpty())
    }

    @Test
    fun decodesShellOutputPayload() {
        val output = ConsoleJson.decodeFromString<BackgroundWorkOutput>(
            """{"stdout":"hello\n","stderr":"warn\n","nextCursor":6,"truncated":false,"status":"exited","exitCode":0,"timedOut":false}""",
        )
        assertEquals("hello\n", output.stdout)
        assertEquals("warn\n", output.stderr)
        assertEquals("exited", output.status)
        assertEquals(0, output.exitCode)
    }
}
