package com.console.mobile

import com.console.mobile.data.model.parsePairingUrl
import com.console.mobile.data.model.splitHostAndCode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The pairing URL is a credential, so it is parsed by one strict parser: the
 * Rust one in `console-shared`, tested in `pairing_test.rs`, which the desktop
 * uses too. These cover only what is Android's own: what the app does when
 * that library is not loaded (a JVM run, a fresh clone, an unsupported ABI).
 *
 * The rule: **unavailable never reads as valid.** A paste is refused, not
 * parsed leniently, because a lenient parser is how a malformed paste becomes a
 * pairing with the wrong machine.
 */
class PairingUrlTest {
    @Test
    fun aPasteIsRefusedWhenTheStrictParserIsUnavailable() {
        if (SharedNative.available) return
        val result = parsePairingUrl("http://192.168.1.10:3000/console/pair#v=1&server=s&code=abc")
        assertTrue("must not be accepted without the strict parser", result.isFailure)
    }

    @Test
    fun splittingDegradesToAnAddressWithNoCode() {
        if (SharedNative.available) return
        // The user can still type the code into its own field; nothing is guessed.
        assertEquals("host:3000#abc" to "", splitHostAndCode("  host:3000#abc  "))
    }

    @Test
    fun whenLoadedTheFacadeAgreesWithTheRustParser() {
        if (!SharedNative.available) return
        val parsed = parsePairingUrl("http://192.168.1.10:3000/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc").getOrThrow()
        assertEquals("http://192.168.1.10:3000", parsed.baseUrl)
        assertEquals("481e2796", parsed.serverId)
        assertEquals("vw9g-zu89-aphc", parsed.code)
        assertEquals("192.168.1.10:3000" to "vw9g-zu89-aphc", splitHostAndCode("http://192.168.1.10:3000/console/pair#v=1&server=s&code=vw9g-zu89-aphc"))
    }
}
