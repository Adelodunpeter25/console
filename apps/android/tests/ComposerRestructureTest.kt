package com.console.mobile

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.contextPercent
import com.console.mobile.core.chat.effectiveThinkingLevel
import com.console.mobile.core.chat.nextThinkingLevel
import com.console.mobile.core.chat.thinkingStepLabel
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class ComposerRestructureTest {
    // Levels are whatever the backend reports for the model — none are assumed.
    @Test
    fun effectiveLevelPrefersSavedThenBackendDefault() {
        val supported = listOf("low", "medium", "high")
        assertEquals("high", effectiveThinkingLevel(supported, "high", "low"))
        // Saved level the model no longer offers: fall back to the backend's default.
        assertEquals("medium", effectiveThinkingLevel(supported, "max", "medium"))
        assertEquals("low", effectiveThinkingLevel(supported, null, null))
        assertNull(effectiveThinkingLevel(emptyList(), "high", "medium"))
    }

    @Test
    fun cyclingWalksTheBackendListAndWraps() {
        val supported = listOf("minimal", "low", "medium")
        assertEquals("low", nextThinkingLevel(supported, "minimal"))
        assertEquals("minimal", nextThinkingLevel(supported, "medium"))
        assertEquals("low", nextThinkingLevel(supported, "not-in-list"))
        assertNull(nextThinkingLevel(emptyList(), "low"))
    }

    @Test
    fun stepLabelCountsNoneAsZeroLikeDesktop() {
        assertEquals("0/6", thinkingStepLabel(listOf("none", "minimal", "low", "medium", "high", "xhigh", "max"), "none"))
        assertEquals("3/6", thinkingStepLabel(listOf("none", "minimal", "low", "medium", "high", "xhigh", "max"), "medium"))
        assertEquals("1/3", thinkingStepLabel(listOf("low", "medium", "high"), "low"))
        assertNull(thinkingStepLabel(listOf("low"), "high"))
    }

    @Test
    fun contextPercentClampsAndHandlesUnknown() {
        assertEquals(42, contextPercent(0, 0, 42.7))
        assertEquals(50, contextPercent(500, 1000, 0.0))
        assertEquals(100, contextPercent(0, 0, 250.0))
        assertNull(contextPercent(0, 0, 0.0))
    }

    @Test
    fun contextUpdateFrameFeedsTheRing() {
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"contextUpdate","context":{"usedTokens":1200,"contextWindow":8000,"percentUsed":15.0,"thresholdRatio":0.8,"modelId":"m","provider":"p","source":"estimate"}}""",
        )
        val after = applyChatEvent(ChatSessionState(), event)
        val ctx = assertNotNull(after.context).let { after.context!! }
        assertEquals(1200, ctx.used_tokens)
        assertEquals(8000, ctx.context_window)
        assertEquals(15.0, ctx.percent_used, 0.0)
    }

    @Test
    fun malformedContextFrameLeavesRingUntouched() {
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>("""{"type":"contextUpdate","context":"nope"}""")
        assertNull(applyChatEvent(ChatSessionState(), event).context)
    }
}
