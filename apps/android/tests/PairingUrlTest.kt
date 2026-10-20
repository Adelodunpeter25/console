package com.console.mobile

import com.console.mobile.data.model.pairingCode
import com.console.mobile.data.model.pairingHost
import com.console.mobile.data.model.parsePairingUrl
import com.console.mobile.data.model.splitHostAndCode
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
            "http://192.168.1.10:3000/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc"
        ).getOrThrow()
        assertEquals("http://192.168.1.10:3000", parsed.baseUrl)
        assertEquals("481e2796", parsed.serverId)
        assertEquals("vw9g-zu89-aphc", parsed.code)
    }

    @Test
    fun parsesAnHttpsPairingUrl() {
        val parsed = parsePairingUrl("https://server.example/console/pair#v=1&server=s&code=c")
            .getOrThrow()
        assertEquals("https://server.example", parsed.baseUrl)
    }

    @Test
    fun keepsTheCodeOutOfTheAddress() {
        val parsed = parsePairingUrl("http://host:3000/console/pair#v=1&server=s&code=secret")
            .getOrThrow()
        assertTrue(
            "the code must not end up in the address: ${parsed.baseUrl}",
            !parsed.baseUrl.contains("secret"),
        )
    }

    @Test
    fun readsTheHostAndCodeSeparately() {
        val url = "http://192.168.1.10:3000/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc"
        assertEquals("192.168.1.10:3000", pairingHost(url))
        assertEquals("vw9g-zu89-aphc", pairingCode(url))
    }

    @Test
    fun splitsAHostAndCodeFromOneLine() {
        // The one-liner a user pastes into the host field.
        val (host, code) = splitHostAndCode("192.168.1.10:3000#vw9g-zu89-aphc")
        assertEquals("192.168.1.10:3000", host)
        assertEquals("vw9g-zu89-aphc", code)
    }

    @Test
    fun splitsTheFullUrlIntoAHostAndCode() {
        val (host, code) =
            splitHostAndCode("http://192.168.1.10:3000/console/pair#v=1&server=481e2796&code=vw9g-zu89-aphc")
        assertEquals("192.168.1.10:3000", host)
        assertEquals("vw9g-zu89-aphc", code)
    }

    @Test
    fun splitsAPlainAddressWithNoCode() {
        val (host, code) = splitHostAndCode("192.168.1.10:3000")
        assertEquals("192.168.1.10:3000", host)
        assertEquals("", code)
    }

    @Test
    fun acceptsAPercentEncodedCode() {
        val parsed = parsePairingUrl("http://host:3000/console/pair#v=1&server=s&code=a%2Bb")
            .getOrThrow()
        assertEquals("a+b", parsed.code)
    }

    @Test
    fun rejectsMalformedUrls() {
        val bad = listOf(
            "",
            "   ",
            // No fragment at all: nothing to redeem.
            "http://host:3000/console/pair",
            // A scheme we do not speak.
            "ftp://host/console/pair#v=1&server=s&code=c",
            // No host.
            "http:///console/pair#v=1&server=s&code=c",
            // A version this app does not know.
            "http://host/console/pair#v=2&server=s&code=c",
            // Missing pieces.
            "http://host/console/pair#v=1&code=",
            "http://host/console/pair#v=1&server=s",
        )
        for (url in bad) {
            assertTrue("should have been rejected: $url", parsePairingUrl(url).isFailure)
        }
    }

    @Test
    fun acceptsAnAddressWithNoScheme() {
        // The one-liner a user pastes: no scheme, just the address and the
        // code.
        val parsed = parsePairingUrl("192.168.1.10:3000#vw9g-zu89-aphc").getOrThrow()
        assertEquals("http://192.168.1.10:3000", parsed.baseUrl)
        assertEquals("vw9g-zu89-aphc", parsed.code)
    }
}
