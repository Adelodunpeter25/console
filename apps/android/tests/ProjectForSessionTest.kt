package com.console.mobile

import com.console.mobile.core.chat.projectForSession
import console.v1.ProjectInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Turning a chat into a worktree chat moves its cwd to the worktree folder,
 * which sits outside the project's path. The project chip used to find the
 * project by folder alone and so fell back to "No project".
 */
class ProjectForSessionTest {
    private val console = ProjectInfo(id = "p1", name = "console", path = "/Users/me/console")
    private val other = ProjectInfo(id = "p2", name = "other", path = "/Users/me/other")
    private val projects = listOf(console, other)

    @Test
    fun worktreeChatKeepsItsProjectThroughTheServerLink() {
        val worktreeCwd = "/Users/me/.console/worktrees/clever-charleston"
        assertEquals(console, projectForSession(projects, "p1", worktreeCwd))
    }

    @Test
    fun folderStillMatchesWhenThereIsNoProjectLink() {
        assertEquals(other, projectForSession(projects, null, "/Users/me/other/apps/x"))
        assertEquals(console, projectForSession(projects, "", "/Users/me/console"))
    }

    @Test
    fun linkWinsOverAMisleadingFolder() {
        assertEquals(other, projectForSession(projects, "p2", "/Users/me/console"))
    }

    @Test
    fun genuinelyProjectlessChatStaysProjectless() {
        assertNull(projectForSession(projects, null, "/tmp/scratch/abc"))
        assertNull(projectForSession(projects, "gone", null))
    }
}
