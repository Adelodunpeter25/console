package com.console.mobile

import com.console.mobile.core.chat.sessionBranch
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class WorktreeBranchTest {
    private val projectBranches = mapOf("p1" to "main")

    @Test
    fun worktreeChatShowsItsOwnBranchNotTheProjects() {
        assertEquals("clever-charleston", sessionBranch("clever-charleston", "p1", projectBranches))
    }

    @Test
    fun plainChatShowsTheProjectsBranch() {
        assertEquals("main", sessionBranch(null, "p1", projectBranches))
        assertEquals("main", sessionBranch("", "p1", projectBranches))
    }

    @Test
    fun unknownProjectHasNoBranch() {
        assertNull(sessionBranch(null, "p9", projectBranches))
        assertNull(sessionBranch(null, null, projectBranches))
    }
}
