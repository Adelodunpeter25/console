package com.console.mobile

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.buildGroupedProjectSections
import com.console.mobile.data.store.ChatStateHolder
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A chat with an unsent draft kept reappearing under "Drafts" after deletion:
 * the list builds a draft row from local state even when the server has no
 * such session, so the local draft has to go with the session.
 */
class DraftDeletionTest {
    private fun draft() = ChatSessionState(input = "half-written thought", draftUpdatedAt = 5L)

    @Test
    fun leftoverDraftResurrectsADeletedSession() {
        // Server no longer lists it, but the local draft is still held.
        val sections = buildGroupedProjectSections(emptyList(), emptyList(), mapOf("gone" to draft()))
        assertEquals(listOf("Drafts"), sections.map { it.projectName })
    }

    @Test
    fun removingTheSessionStateClearsTheDraftRow() {
        val chats = ChatStateHolder(mapOf("gone" to draft()))
        chats.remove("gone")
        assertTrue("gone" !in chats.sessions.value)
        val sections = buildGroupedProjectSections(emptyList(), emptyList(), chats.sessions.value)
        assertTrue(sections.isEmpty())
    }

    @Test
    fun removeLeavesOtherSessionsAlone() {
        val chats = ChatStateHolder(mapOf("gone" to draft(), "kept" to draft()))
        chats.remove("gone")
        assertEquals(setOf("kept"), chats.sessions.value.keys)
    }
}
