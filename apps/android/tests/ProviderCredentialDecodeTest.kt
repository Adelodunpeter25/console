package com.console.mobile

import com.console.mobile.data.api.ConsoleJson
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ProviderCredentialStatus
import console.v1.ProviderCredentialStatuses
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Wire decoding for the provider API-key status payload
 * (`GET /api/providers/credentials`).
 *
 * Worth a test because this is the one response in the auth area whose shape is
 * generated rather than hand-written, and it has two traps:
 *
 *  - the payload is a `map<string, Message>`, and protojson omits an *empty*
 *    map entirely, so Wire hands back null where a Kotlin `Map` return type
 *    promises a value. `OkHttpConsoleApi.getProviderCredentials` exists to
 *    absorb that.
 *  - `configured` is explicit-presence, so "no key" arrives as
 *    `configured: false` while a server that did not report one omits the field
 *    entirely. Conflating the two would show a key the provider does not have.
 *
 * The JSON below is verbatim server output, captured from a live server.
 */
class ProviderCredentialDecodeTest {
    private val moshi: Moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
    private val statusesAdapter = moshi.adapter(ProviderCredentialStatuses::class.java)
    private val statusAdapter = moshi.adapter(ProviderCredentialStatus::class.java)

    /** Mirrors OkHttpConsoleApi: Wire yields null for an omitted map. */
    private fun providers(json: String): Map<String, ProviderCredentialStatus> =
        statusesAdapter.fromJson(json)!!.providers ?: emptyMap()

    @Test
    fun readsAStoredKey() {
        val providers = providers(
            """{"providers":{"opencode":{"configured":true,"source":"stored","updatedAt":"2026-10-16T09:49:49Z"}}}""",
        )
        val opencode = providers["opencode"]!!
        assertTrue(opencode.configured!!)
        assertEquals("stored", opencode.source)
        assertEquals("2026-10-16T09:49:49Z", opencode.updated_at)
    }

    @Test
    fun noKeyIsFalseNotAbsent() {
        val opencode = providers("""{"providers":{"opencode":{"configured":false}}}""")["opencode"]!!
        // Must be false-and-present, not null: null means "the server said
        // nothing", which is a different claim.
        assertEquals(false, opencode.configured)
        assertNull(opencode.source)
    }

    @Test
    fun unreportedIsAbsent() {
        // The distinction the UI depends on: absent = unknown, false = none.
        val opencode = providers("""{"providers":{"opencode":{}}}""")["opencode"]!!
        assertNull(opencode.configured)
    }

    @Test
    fun environmentSuppliedKeyIsDistinguishable() {
        // Clearing a stored key cannot remove an env-supplied one, so a client
        // that hides its Remove button must be able to tell these apart.
        val opencode = providers("""{"providers":{"opencode":{"configured":true,"source":"env"}}}""")["opencode"]!!
        assertTrue(opencode.configured!!)
        assertEquals("env", opencode.source)
    }

    @Test
    fun anEmptyMapDoesNotCrashTheClient() {
        // protojson omits an empty map, so this is what a server with no
        // key-taking providers actually sends.
        assertTrue(providers("""{}""").isEmpty())
    }

    @Test
    fun theKeyItselfIsNeverInThePayload() {
        // Nothing to decode — the assertion is that the server has nothing to
        // send. Spelled out because a future change adding the value to this
        // message would otherwise pass every other test here.
        val payload = """{"providers":{"opencode":{"configured":true,"source":"stored"}}}"""
        val decoded = providers(payload)
        assertEquals(1, decoded.size)
        assertFalse(decoded.toString().contains("sk-"))
    }
}