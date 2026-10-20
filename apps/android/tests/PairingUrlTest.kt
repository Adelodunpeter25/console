package com.console.mobile

import com.console.mobile.data.model.parsePairingUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The pairing URL is the one string a user pastes to connect a device.
 *
 * The grant is a credential, so the parser is strict: anything it cannot
 * read exactly is rejected rather than repaired, and the grant must never
 * end up in the address the app will talk to.
 */
class PairingUrlTest {
    @Test
    fun parsesAPairingUrl() {
        val parsed = parsePairingUrl(
            "http://192.168.1.10:3000/console/pair#v=1&server=srv-1&grant=cp1_abc"
        ).getOrThrow()
        assertEquals("http://192.168.1.10:3000", parsed.baseUrl)
        assertEquals("srv-1", parsed.serverId)
        assertEquals("cp1_abc", parsed.grant)
    }

    @Test
    fun parsesAnHttpsPairingUrl() {
        val parsed = parsePairingUrl("https://server.example/console/pair#v=1&server=s&grant=g")
            .getOrThrow()
        assertEquals("https://server.example", parsed.baseUrl)
    }

    @Test
    fun keepsTheGrantOutOfTheAddress() {
        val parsed = parsePairingUrl("http://host:3000/console/pair#v=1&server=s&grant=secret")
            .getOrThrow()
        assertTrue(
            "the grant must not end up in the address: ${parsed.baseUrl}",
            !parsed.baseUrl.contains("secret"),
        )
    }

    @Test
    fun acceptsAPercentEncodedGrant() {
        val parsed = parsePairingUrl("http://host:3000/console/pair#v=1&server=s&grant=a%2Bb")
            .getOrThrow()
        assertEquals("a+b", parsed.grant)
    }

    @Test
    fun rejectsMalformedUrls() {
        val bad = listOf(
            "",
            "   ",
            // No fragment at all: nothing to redeem.
            "http://host:3000/console/pair",
            // No scheme.
            "host:3000/console/pair#v=1&server=s&grant=g",
            // A scheme we do not speak.
            "ftp://host/console/pair#v=1&server=s&grant=g",
            // No host.
            "http:///console/pair#v=1&server=s&grant=g",
            // A version this app does not know.
            "http://host/console/pair#v=2&server=s&grant=g",
            // Missing pieces.
            "http://host/console/pair#v=1&grant=g",
            "http://host/console/pair#v=1&server=s",
            "http://host/console/pair#server=s&grant=g",
        )
        for (url in bad) {
            assertTrue("should have been rejected: $url", parsePairingUrl(url).isFailure)
        }
    }
}
