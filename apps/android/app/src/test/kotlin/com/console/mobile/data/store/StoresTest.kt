package com.console.mobile.data.store

import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SessionStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class StoresTest {
    @Test fun appStateTabAndSelections() {
        val h = AppStateHolder()
        h.setActiveTab(MobileTab.Chat)
        assertEquals(MobileTab.Chat, h.state.value.activeTab)
        assertEquals(MobileTab.Home, h.state.value.previousTab)
        h.openChatSession("s1")
        assertEquals("s1", h.state.value.selectedSessionId)
        h.clearSelections()
        assertNull(h.state.value.selectedSessionId)
    }

    @Test fun sessionStatusesSeedDoesNotOverwrite() {
        val h = SessionStateHolder()
        h.setStatus("a", SessionStatus.Working)
        h.setStatusesSeed(mapOf("a" to SessionStatus.Idle, "b" to SessionStatus.Done))
        assertEquals(SessionStatus.Working, h.statuses.value["a"])
        assertEquals(SessionStatus.Done, h.statuses.value["b"])
        h.clearStatus("a")
        assertNull(h.statuses.value["a"])
    }

    @Test fun projectLists() {
        val h = ProjectStateHolder()
        val p = ProjectInfo("p1", "n", "/r", 1, 2)
        h.addProject(p)
        h.removeProject("p1")
        assertTrue(h.state.value.projects.isEmpty())
        val s = SessionHeader("s", "t", "/c", null, "m", "opencode", 1, 2)
        h.prependSession(s)
        h.removeSession("s")
        assertTrue(h.state.value.sessions.isEmpty())
    }

    @Test fun terminalFindLive() {
        val h = TerminalStateHolder()
        h.ensure(TerminalRecord("t1", "p1", TerminalStatus.Running, cwd = "/a"))
        h.ensure(TerminalRecord("t2", "p1", TerminalStatus.Exited, cwd = "/a"))
        assertEquals("t1", h.findLive("p1", "/a"))
        h.appendOutput("t1", "hi")
        assertEquals("hi", h.buffers.value["t1"])
        h.remove("t1")
        assertNull(h.findLive("p1", "/a"))
    }

    @Test fun chatHolderUpdate() {
        val h = ChatStateHolder()
        h.setInput("s", "hello")
        assertEquals("hello", h.get("s").input)
        h.clear("s")
        assertEquals("", h.get("s").input)
    }

    @Test fun providerResolve() {
        val h = ProviderStateHolder()
        h.setProviders(listOf(com.console.mobile.data.model.ProviderCatalogEntry("opencode", "OC", "d", listOf(com.console.mobile.data.model.Model("m1", "opencode", 100)), "none")))
        assertEquals("opencode", h.resolveProvider("m1", null))
        assertEquals("fallback", h.resolveProvider("unknown", "fallback"))
    }
}
