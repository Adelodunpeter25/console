package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.IntegrationStatus
import console.v1.IntegrationsResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for binary integration status. The fixture lives in
 * proto/testdata/integrations and is shared by Go, Rust, and Kotlin.
 */
class IntegrationsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val responseAdapter = moshi.adapter(IntegrationsResponse::class.java)
    private val rowAdapter = moshi.adapter(IntegrationStatus::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/integrations/$name").readText().trim()

    @Test
    fun decodesGoldenResponseFixture() {
        val response = responseAdapter.fromJson(fixture("response.json"))!!
        assertEquals(1, response.integrations.size)
        val gh = response.integrations[0]
        assertEquals("gh", gh.id)
        assertEquals("GitHub CLI", gh.name)
        // `installed` has explicit presence on the wire, so it decodes to
        // Boolean? rather than Boolean.
        assertEquals(true, gh.installed)
        assertEquals("2.75.0", gh.version)
        assertEquals("/opt/homebrew/bin/gh", gh.binary_path)
        assertTrue(gh.install_url.isNotEmpty())
        assertTrue(gh.install_hint.isNotEmpty())
    }

    @Test
    fun missingBinaryCarriesNoVersionOrPath() {
        val row = rowAdapter.fromJson(
            """{"id":"gh","name":"GitHub CLI","description":"d","installed":false,"installUrl":"https://cli.github.com","installHint":"sudo apt install gh"}"""
        )!!
        assertEquals(false, row.installed)
        assertNull(row.version)
        assertNull(row.binary_path)
    }
}
