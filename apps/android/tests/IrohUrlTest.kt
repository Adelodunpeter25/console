package com.console.mobile

import com.console.mobile.core.util.irohNodeId
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.reachableUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class IrohUrlTest {
    private val node = "ab12".repeat(16)

    @Test
    fun anIrohAddressKeepsItsSchemeWhenNormalised() {
        assertEquals("console-iroh://$node", normalizeBackendUrl("console-iroh://$node/"))
    }

    @Test
    fun aBadNodeIdIsRefused() {
        assertNull(normalizeBackendUrl("console-iroh://example.com"))
        assertNull(normalizeBackendUrl("console-iroh://${node.uppercase()}"))
        assertNull(irohNodeId("http://192.168.1.10:3000"))
    }

    @Test
    fun theNodeIdComesOutOfAFullPairingUrl() {
        assertEquals(node, irohNodeId("console-iroh://$node/console/pair#v=1&server=s&code=c"))
    }

    @Test
    fun anHttpAddressIsLeftAlone() {
        assertEquals("http://192.168.1.10:3000", reachableUrl("http://192.168.1.10:3000"))
    }

    @Test
    fun withoutTheNativeLibraryAnIrohAddressIsNotRewritten() {
        if (SharedNative.available) return
        // No tunnel can be made: the address stays, and the request fails loudly.
        assertEquals("console-iroh://$node", reachableUrl("console-iroh://$node"))
    }
}
