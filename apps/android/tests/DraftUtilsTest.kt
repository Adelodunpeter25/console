package com.console.mobile.core.chat

import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UserMessage
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class DraftUtilsTest {
    @Test fun testIsDraftSession() {
        assertFalse(isDraftSession(createChatSessionState()))
        assertTrue(isDraftSession(createChatSessionState().copy(input = "  hi  ")))
        assertTrue(isDraftSession(createChatSessionState().copy(attachments = listOf(ImageAttachment("a", "b")))))
    }

    @Test fun testTrimDraftAttachments() {
        val list = listOf(
            ImageAttachment("1", "png"),
            ImageAttachment("2", "png"),
            ImageAttachment("3", "png"),
        )
        val trimmed = trimDraftAttachments(list)
        assertEquals(2, trimmed.size)
        assertEquals("2", trimmed[0].data)
        assertEquals("3", trimmed[1].data)
    }

    @Test fun testDraftPreview() {
        assertEquals("Draft", draftPreview(createChatSessionState()))
        assertEquals("Draft: hello world", draftPreview(createChatSessionState().copy(input = "hello   world")))
        assertEquals("Draft: 1 image", draftPreview(createChatSessionState().copy(attachments = listOf(ImageAttachment("1", "png")))))
        assertEquals("Draft: 2 images", draftPreview(createChatSessionState().copy(attachments = listOf(ImageAttachment("1", "png"), ImageAttachment("2", "png")))))
    }

    @Test fun testFormatProjectTitle() {
        assertEquals("My Cool App", formatProjectTitle("my_cool-app"))
        assertEquals("Console", formatProjectTitle("console"))
        assertEquals("", formatProjectTitle(""))
    }

    @Test fun testBuildGroupedProjectSections() {
        val projects = listOf(
            ProjectInfo("p1", "Project 1", "/repos/p1", 1, 2),
            ProjectInfo("p2", "Project 2", "/repos/p2", 1, 2),
        )

        val s1 = SessionHeader("s1", "Session 1", "/repos/p1", "p1", "m", "codex", 1, 100, 1)
        val s2 = SessionHeader("s2", "Session 2", "/repos/p2", "p2", "m", "codex", 1, 200, 1)
        val sGeneral = SessionHeader("sg", "General Session", "/repos/p1", null, "m", "codex", 1, 300, 1)

        val draftSession = createChatSessionState().copy(
            input = "draft for new chat",
            draftUpdatedAt = 400L,
        )

        val sections = buildGroupedProjectSections(
            sessions = listOf(s1, s2, sGeneral),
            projects = projects,
            draftSessions = mapOf("s_draft" to draftSession),
            searchQuery = "",
        )

        // Sections order: Drafts first, then sorted by latestAt
        assertEquals("Drafts", sections[0].projectName)
        assertEquals(1, sections[0].data.size)
        assertEquals("Draft: draft for new chat", sections[0].data[0].title)

        // Rest of the sections
        val otherSectionNames = sections.drop(1).map { it.projectName }
        assertTrue(otherSectionNames.contains("General"))
        assertTrue(otherSectionNames.contains("Project 1"))
        assertTrue(otherSectionNames.contains("Project 2"))
    }
}
