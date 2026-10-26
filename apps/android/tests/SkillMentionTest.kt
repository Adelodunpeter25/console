package com.console.mobile

import com.console.mobile.core.util.parseSkillMentions
import org.junit.Assert.assertEquals
import org.junit.Test

/** Parsing /skill mentions in composer and bubble text. */
class SkillMentionTest {
    @Test
    fun parsesSkillMentionsAtWordBoundaries() {
        val mentions = parseSkillMentions("run /systematic-debugging now, or /plan.")
        assertEquals(2, mentions.size)
        assertEquals("systematic-debugging", mentions[0].name)
        assertEquals(4..24, mentions[0].range)
        assertEquals("plan", mentions[1].name)
        assertEquals(34..38, mentions[1].range)
    }

    @Test
    fun ignoresPathSlashesAndMidWordSlashes() {
        val mentions = parseSkillMentions("visit a/b/c or and/or but /my-skill is fine")
        assertEquals(1, mentions.size)
        assertEquals("my-skill", mentions[0].name)
    }
}
