package com.console.mobile

import com.console.mobile.core.chat.isGenericTitle
import com.console.mobile.core.chat.isSessionTitlePending
import console.v1.SessionHeader
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TitlePendingTest {
    private fun pending(title: String, messageCount: Int): Boolean =
        isSessionTitlePending(
            SessionHeader(
                id = "s1",
                title = title,
                message_count = messageCount,
                status = "done",
            ),
        )

    @Test
    fun placeholderListMatchesServer() {
        assertTrue(isGenericTitle(""))
        assertTrue(isGenericTitle("  New Chat  "))
        assertTrue(isGenericTitle("New Session"))
        assertTrue(isGenericTitle("New mobile session"))
        assertTrue(isGenericTitle("Untitled"))
        assertFalse(isGenericTitle("manual rename"))
    }

    @Test
    fun freshEmptySessionIsNotPending() {
        assertFalse(pending("New Chat", 0))
    }

    @Test
    fun pendingAfterFirstPrompt() {
        assertTrue(pending("New Session", 1))
        assertTrue(pending("New Chat", 2))
    }

    @Test
    fun generatedTitleIsNotPending() {
        assertFalse(pending("Fix flaky spec", 1))
    }
}
