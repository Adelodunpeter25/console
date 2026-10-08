package com.console.mobile

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.contextPercent
import com.console.mobile.core.chat.thinkingChipLabel
import com.console.mobile.core.chat.thinkingOptions
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class ComposerRestructureTest {
    @Test
    fun levelsReadAsWordsNeverRawEnums() {
        val opts = thinkingOptions(listOf("low", "medium", "high", "xhigh", "max"))
        assertEquals(listOf("Fast", "Balanced", "Deep", "Extra deep", "Max"), opts.map { it.label })
        assertEquals("Maximum depth", opts.last().description)
        // Wire values stay raw.
        assertEquals(listOf("low", "medium", "high", "xhigh", "max"), opts.map { it.value })
    }

    @Test
    fun unknownLevelIsKeptWithReadableLabel() {
        assertEquals("Turbo", thinkingOptions(listOf("turbo")).single().label)
    }

    @Test
    fun chipHidesWhenModelReportsNoLevels() {
        assertNull(thinkingChipLabel(emptyList(), "high", "medium"))
    }

    @Test
    fun chipFallsBackToDefaultWhenSavedLevelUnsupported() {
        // Saved "max" isn't offered by this model, so show its default.
        assertEquals("Balanced", thinkingChipLabel(listOf("low", "medium"), "max", "medium"))
        assertEquals("Fast", thinkingChipLabel(listOf("low", "medium"), "low", "medium"))
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
