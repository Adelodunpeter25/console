package com.console.mobile

import com.console.mobile.data.model.toUi
import console.v1.AgentMessage
import console.v1.AgentUserMessage
import console.v1.BackgroundWork as WireBackgroundWork
import console.v1.SessionBootstrapResponse
import console.v1.SessionHeader
import console.v1.TodoItem
import org.junit.Assert.assertEquals
import org.junit.Test

class BinaryBootstrapTest {
    @Test
    fun decodesTheBinaryBootstrapEnvelope() {
        val wire = SessionBootstrapResponse(
            header_ = SessionHeader(id = "s1", title = "Demo"),
            messages = listOf(
                AgentMessage(message = AgentMessage.Message.User(AgentUserMessage(content = "Hello"))),
            ),
            todos = listOf(TodoItem(id = 1, content = "task", status = "pending")),
            background_work = listOf(
                WireBackgroundWork(
                    id = "w1",
                    session_id = "s1",
                    kind = "shell",
                    status = "running",
                    created_at = "now",
                ),
            ),
        )

        val decoded = SessionBootstrapResponse.ADAPTER.decode(
            SessionBootstrapResponse.ADAPTER.encodeByteString(wire),
        )

        assertEquals("s1", decoded.header_?.id)
        assertEquals(1, decoded.messages.size)
        val user = decoded.messages[0].message as AgentMessage.Message.User
        assertEquals("Hello", user.value.content)
        assertEquals("task", decoded.todos[0].content)
        val work = decoded.background_work[0].toUi()
        assertEquals("w1", work.id)
        assertEquals("s1", work.sessionId)
        assertEquals(true, work.isRunning())
    }

    @Test
    fun decodesBinaryHistoryPage() {
        val wire = console.v1.SessionMessagesPage(
            header_ = SessionHeader(id = "s2"),
            messages = listOf(
                AgentMessage(message = AgentMessage.Message.User(AgentUserMessage(content = "older"))),
            ),
            has_more = true,
            next_cursor = 42L,
        )
        val decoded = console.v1.SessionMessagesPage.ADAPTER.decode(
            console.v1.SessionMessagesPage.ADAPTER.encodeByteString(wire),
        )
        assertEquals("s2", decoded.header_?.id)
        val user = decoded.messages[0].message as AgentMessage.Message.User
        assertEquals("older", user.value.content)
        assertEquals(true, decoded.has_more)
        assertEquals(42L, decoded.next_cursor)
    }

    private fun com.console.mobile.data.model.BackgroundWork.isRunning(): Boolean =
        kind == "shell" && status == "running"
}
