package com.console.mobile

import com.console.mobile.core.util.ComposerTrigger
import com.console.mobile.core.util.detectComposerTrigger
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
    fun newlineCountsAsWhitespace() {
        assertEquals(ComposerTrigger.Slash(start = 5, query = "cmd"), trigger("line\n/cmd"))
    }
}
