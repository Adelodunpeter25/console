package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.model.INPUT_FRAME_TAG
import com.console.mobile.data.model.OUTPUT_FRAME_TAG
import com.console.mobile.data.model.TerminalSpawnParams
import com.console.mobile.data.model.buildTerminalWsUrl
import com.console.mobile.data.store.TerminalRecord
import com.console.mobile.data.store.TerminalStateHolder
import com.console.mobile.data.store.TerminalStatus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

class TerminalRepositoryTest {
    @Test fun testBuildTerminalWsUrl() {
        val params = TerminalSpawnParams(
            cwd = "/repos/my-proj",
            cols = 120,
            rows = 40,
            label = "My Terminal",
            shell = "/bin/zsh",
            proto = "binary",
        )
        val url = buildTerminalWsUrl("http://localhost:3000", params)
        assertTrue(url.startsWith("ws://localhost:3000/api/terminals?"))
        assertTrue(url.contains("cols=120"))
        assertTrue(url.contains("rows=40"))
        assertTrue(url.contains("proto=binary"))
    }

    @Test fun testBinaryFrameTags() {
        assertEquals(0x01.toByte(), OUTPUT_FRAME_TAG)
        assertEquals(0x01.toByte(), INPUT_FRAME_TAG)
    }

    @Test fun testTerminalStateManagement() {
        val holder = TerminalStateHolder()
        holder.ensure(TerminalRecord("t1", "p1", TerminalStatus.Running))
        holder.appendOutput("t1", "line 1\n")
        holder.appendOutput("t1", "line 2\n")
        assertEquals("line 1\nline 2\n", holder.buffers.value["t1"])
        holder.remove("t1")
        assertTrue(holder.terminals.value.isEmpty())
        assertTrue(holder.buffers.value.isEmpty())
    }
}
