package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.StreamPart
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.UserMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ChatEventsTest {
    @Test fun streamPartAccumulates() {
        val s = ChatSessionState()
        val s2 = applyChatEvent(s, AgentSessionEvent(type = "modelStreamPart", part = StreamPart(text = "hel")))
        val s3 = applyChatEvent(s2, AgentSessionEvent(type = "modelStreamPart", part = StreamPart(text = "lo", thinking = "th")))
        assertEquals("hello", s3.streamingText)
        assertEquals("th", s3.streamingThinking)
    }

    @Test fun streamResetClearsBuffers() {
        val s = ChatSessionState(streamingText = "x", streamingThinking = "y", activeToolCalls = listOf(ToolCall("1", "bash")))
        val s2 = applyChatEvent(s, AgentSessionEvent(type = "streamReset"))
        assertEquals("", s2.streamingText)
        assertEquals("", s2.streamingThinking)
        assertTrue(s2.activeToolCalls.isEmpty())
    }

    @Test fun turnStartAndEnd() {
        var s = applyChatEvent(ChatSessionState(), AgentSessionEvent(type = "turnStart", prompt = "hi"))
        assertTrue(s.running)
        assertEquals(1, s.runs.size)
        s = applyChatEvent(s, AgentSessionEvent(type = "turnEnd", turnId = "t1"))
        assertTrue(!s.running)
        assertEquals(RunStatus.Completed, s.runs[0].status)
    }

    @Test fun modelStreamEndFinalResponse() {
        var s = applyChatEvent(ChatSessionState(), AgentSessionEvent(type = "turnStart", prompt = "hi"))
        s = applyChatEvent(s, AgentSessionEvent(type = "modelStreamPart", part = StreamPart(text = "abc")))
        val turn = AssistantMessage(content = listOf(TextPart("done")))
        s = applyChatEvent(s, AgentSessionEvent(type = "modelStreamEnd", turnId = "t", turn = turn))
        assertEquals(1, s.messages.size)
        assertEquals("", s.streamingText)
    }

    @Test fun toolFlowSetsResult() {
        var s = applyChatEvent(ChatSessionState(), AgentSessionEvent(type = "turnStart", prompt = "hi"))
        val call = ToolCall("c1", "bash")
        val turn = AssistantMessage(content = listOf(TextPart("run"), com.console.mobile.data.model.ToolCallPart(call)))
        s = applyChatEvent(s, AgentSessionEvent(type = "modelStreamEnd", turnId = "t", turn = turn))
        assertEquals(1, s.activeToolCalls.size)
        s = applyChatEvent(s, AgentSessionEvent(type = "toolExecutionStart", calls = listOf(call)))
        s = applyChatEvent(s, AgentSessionEvent(type = "toolExecutionResult", result = ToolResult("c1", "bash")))
        val snap = toChatSnapshot(s)
        assertEquals(1, snap.liveToolResults.size)
        s = applyChatEvent(s, AgentSessionEvent(type = "toolExecutionEnd", results = listOf(ToolResult("c1", "bash"))))
        assertTrue(s.activeToolCalls.isEmpty())
    }

    @Test fun abortErrorMarksAborted() {
        var s = applyChatEvent(ChatSessionState(), AgentSessionEvent(type = "turnStart", prompt = "hi"))
        s = applyChatEvent(s, AgentSessionEvent(type = "error", error = com.console.mobile.data.model.EventError("aborted by user")))
        assertEquals(RunStatus.Aborted, s.runs[0].status)
    }

    @Test fun errorAppendsBubble() {
        var s = applyChatEvent(ChatSessionState(), AgentSessionEvent(type = "turnStart", prompt = "hi"))
        s = applyChatEvent(s, AgentSessionEvent(type = "error", error = com.console.mobile.data.model.EventError("boom")))
        assertEquals(RunStatus.Failed, s.runs[0].status)
        assertEquals(1, s.messages.size)
    }

    @Test fun ensureIdsAssigns() {
        val msgs = listOf(UserMessage(content = "hi"))
        val out = ensureMessageIds(msgs)
        assertTrue(out[0].id != null)
        val again = ensureMessageIds(out)
        assertTrue(again === out)
    }
}
