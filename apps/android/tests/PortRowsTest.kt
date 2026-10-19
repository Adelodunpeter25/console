package com.console.mobile

import com.console.mobile.core.forward.LocalForward
import com.console.mobile.feature.ports.portRows
import console.v1.ForwardedPort
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The ports list is a join of two sources that are free to disagree: the
 * server says what it forwards, this device says what it has a tunnel into.
 * Getting that join wrong would show "Tunneled" on a port nothing can open.
 */
class PortRowsTest {
    @Test
    fun rowsAreOrderedByPort() {
        val rows = portRows(
            listOf(ForwardedPort(port = 5173), ForwardedPort(port = 3000), ForwardedPort(port = 8080)),
            emptyList(),
        )
        assertEquals(listOf(3000, 5173, 8080), rows.map { it.port })
    }

    @Test
    fun aPortWithATunneledDeviceForwardReadsAsTunneled() {
        val rows = portRows(
            listOf(ForwardedPort(port = 5173, url = "http://192.168.1.20:40000")),
            listOf(LocalForward(remotePort = 5173, localPort = 43111, localUrl = "http://127.0.0.1:43111/")),
        )
        assertEquals(1, rows.size)
        assertTrue(rows.single().tunneled)
        assertEquals("http://127.0.0.1:43111/", rows.single().localUrl)
        assertEquals("http://192.168.1.20:40000", rows.single().lanUrl)
    }

    @Test
    fun theServersPortWithoutALocalListenerIsNotTunneled() {
        val rows = portRows(
            listOf(ForwardedPort(port = 5173, url = "http://192.168.1.20:40000")),
            listOf(LocalForward(remotePort = 3000, localPort = 43112, localUrl = "http://127.0.0.1:43112/")),
        )
        assertFalse(rows.single().tunneled)
        assertNull(rows.single().localUrl)
    }

    @Test
    fun blankMetadataIsDroppedRatherThanShown() {
        val rows = portRows(listOf(ForwardedPort(port = 3000, url = "", project_id = "  ")), emptyList())
        assertNull(rows.single().lanUrl)
        assertNull(rows.single().projectId)
    }

    @Test
    fun projectIdIsCarriedThroughForNameLookup() {
        val rows = portRows(listOf(ForwardedPort(port = 3000, project_id = "p-1")), emptyList())
        assertEquals("p-1", rows.single().projectId)
    }
}
