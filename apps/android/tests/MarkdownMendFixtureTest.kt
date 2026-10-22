package com.console.mobile

import com.console.mobile.feature.chat.markdown.closeHanging
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Streaming markdown keeps unfinished inline syntax visually stable. */
class MarkdownMendFixtureTest {
    @Test
    fun closesUnfinishedEmphasisWithoutChangingSource() {
        assertEquals("**hello**", closeHanging("**hello"))
        assertEquals("*hello*", closeHanging("*hello"))
        assertEquals("~~hello~~", closeHanging("~~hello"))
    }

    @Test
    fun closesUnfinishedCodeSpan() {
        assertEquals("`hello`", closeHanging("`hello"))
    }

    @Test
    fun makesUnfinishedLinkDisplayAsAStableLink() {
        assertEquals(
            "[hello](console:pending-link)",
            closeHanging("[hello](https://example"),
        )
    }

    @Test
    fun leavesCompleteMarkdownAlone() {
        assertNull(closeHanging("**hello** and [link](https://example.com)"))
    }

    @Test
    fun guardsAStreamingSetextUnderline() {
        assertEquals("Title\n-\u200B", closeHanging("Title\n-"))
    }
}
