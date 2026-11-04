package com.console.mobile

import com.console.mobile.core.util.ComposerTrigger
import com.console.mobile.core.util.detectComposerTrigger
import com.console.mobile.core.util.filterChatRefs
import com.console.mobile.core.util.slugifyChatTitle
import console.v1.SessionHeader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ComposerTriggerTest {
    private fun trigger(text: String, cursor: Int = text.length): ComposerTrigger? =
        detectComposerTrigger(text, cursor)

    @Test
    fun slashAtStartTriggers() {
        assertEquals(ComposerTrigger.Slash(start = 0, query = ""), trigger("/"))
        assertEquals(ComposerTrigger.Slash(start = 0, query = "wor"), trigger("/wor"))
    }

    @Test
    fun slashAfterWhitespaceTriggersWithRealStart() {
        assertEquals(ComposerTrigger.Slash(start = 6, query = "wor"), trigger("hello /wor"))
    }

    @Test
    fun caretBeforeSlashHasNoTrigger() {
        assertNull(trigger("hello /wor", cursor = 3))
        assertNull(trigger("hello /wor", cursor = 6))
    }

    @Test
    fun slashInsideWordHasNoTrigger() {
        assertNull(trigger("a/b"))
    }

    @Test
    fun textWithoutSlashHasNoTrigger() {
        assertNull(trigger("hello world"))
    }

    @Test
    fun mentionStillTriggersMidMessage() {
        assertEquals(ComposerTrigger.Mention(start = 6, query = "Com"), trigger("hello @Com"))
    }

    @Test
    fun nearestCommandWins() {
        assertEquals(ComposerTrigger.Slash(start = 11, query = "two"), trigger("hello /one /two"))
    }

    @Test
    fun commandQueryOnlyAcceptsCommandCharacters() {
        assertNull(trigger("hello /foo/bar"))
        assertNull(trigger("hello /foo bar"))
        assertEquals(
            ComposerTrigger.Slash(start = 6, query = "foo-bar:baz_9"),
            trigger("hello /foo-bar:baz_9"),
        )
    }

    @Test
    fun chatRefTriggersAfterBoundary() {
        assertEquals(ComposerTrigger.Chat(start = 0, query = ""), trigger("$"))
        assertEquals(ComposerTrigger.Chat(start = 6, query = "fix-login"), trigger("hello \$fix-login"))
        assertEquals(ComposerTrigger.Chat(start = 7, query = "fix-log"), trigger("hello (\$fix-log"))
    }

    @Test
    fun chatRefSkipsPricesAndMidWordDollar() {
        assertNull(trigger("it costs \$5"))
        assertNull(trigger("it costs \$500"))
        assertNull(trigger("a\$b"))
        assertNull(trigger("hello \$foo bar"))
    }

    @Test
    fun slugifyMatchesServer() {
        assertEquals("fix-login-bug", slugifyChatTitle("Fix Login Bug"))
        assertEquals("what-s-this", slugifyChatTitle("  What's -- this!! "))
        assertEquals("", slugifyChatTitle("!!!"))
    }

    @Test
    fun chatRefFilterExcludesCurrentAndMatchesSlug() {
        val sessions = listOf(
            SessionHeader(id = "current-1", title = "Current Chat", status = "idle"),
            SessionHeader(id = "abc-123", title = "Fix Login Bug", status = "idle"),
            SessionHeader(id = "xyz-999", title = "Other Work", status = "idle"),
        )
        assertEquals(listOf("abc-123"), filterChatRefs(sessions, "current-1", "fix").map { it.id })
        assertEquals(listOf("abc-123"), filterChatRefs(sessions, "current-1", "abc").map { it.id })
        assertEquals(2, filterChatRefs(sessions, "current-1", "").size)
    }

    @Test
    fun newlineCountsAsWhitespace() {
        assertEquals(ComposerTrigger.Slash(start = 5, query = "cmd"), trigger("line\n/cmd"))
    }
}
