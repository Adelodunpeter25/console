package com.console.mobile

import com.console.mobile.core.util.mentionPaths
import com.console.mobile.core.util.parseChatMentions
import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

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

    @Test
    fun mentionPathsCarrySkillsAndChatRefsButNotSlashPaths() {
        val paths = mentionPaths("/review \$fix-bug @a.kt see /usr/bin")
        assertEquals(listOf("a.kt", "skill:review", "chat:fix-bug"), paths)
    }

    @Test
    fun chatRefsMatchSharedServerFixture() {
        val type = Types.newParameterizedType(
            List::class.java,
            Types.newParameterizedType(Map::class.java, String::class.java, Any::class.java),
        )
        val cases = Moshi.Builder().build().adapter<List<Map<String, Any>>>(type)
            .fromJson(File("../../../proto/testdata/session/chat_refs.json").readText())!!
        assertEquals(true, cases.isNotEmpty())
        for (case in cases) {
            val text = case["text"] as String
            @Suppress("UNCHECKED_CAST")
            val want = case["refs"] as List<String>
            assertEquals(text, want, parseChatMentions(text).map { it.slug })
        }
    }
}
