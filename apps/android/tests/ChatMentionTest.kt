package com.console.mobile

import com.console.mobile.core.util.parseChatMentions
import org.junit.Assert.assertEquals
import org.junit.Test

/** Parsing $ chat-ref mentions in composer text. */
class ChatMentionTest {
    @Test
    fun parsesChatRefsAtWordBoundaries() {
        val mentions = parseChatMentions("use \$fix-login-bug and \$visualize-codebase-lines")
        assertEquals(2, mentions.size)
        assertEquals("fix-login-bug", mentions[0].slug)
        assertEquals(4..17, mentions[0].range)
        assertEquals("visualize-codebase-lines", mentions[1].slug)
    }

    @Test
    fun ignoresPricesAndMidWordDollars() {
        val mentions = parseChatMentions("costs \$5 and a\$b test")
        assertEquals(0, mentions.size)
    }
}
