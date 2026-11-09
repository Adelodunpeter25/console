package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SetSttConfigResponse
import console.v1.SttStatus
import console.v1.SttTranscript
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for speech-to-text. Fixtures live in
 * proto/testdata/stt and are shared by Go, Rust, and Kotlin.
 */
class SttFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val statusAdapter = moshi.adapter(SttStatus::class.java)
    private val setConfigAdapter = moshi.adapter(SetSttConfigResponse::class.java)
    private val transcriptAdapter = moshi.adapter(SttTranscript::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/stt/$name").readText().trim()

    @Test
    fun decodesGoldenConfiguredFixture() {
        val msg = statusAdapter.fromJson(fixture("status_configured.json"))!!
        // `configured` has explicit presence (optional bool): Some(true) when set.
        assertEquals(true, msg.configured)
        assertEquals("stored", msg.source)
        assertEquals("e32ab932dec1b35e0df9c0ec56ab6c8f", msg.account_id)
        assertEquals("@cf/openai/whisper-large-v3-turbo", msg.model)
        assertEquals("2026-11-09T10:30:00Z", msg.updated_at)
        assertEquals(4194304, msg.max_audio_bytes)
    }

    @Test
    fun decodesGoldenUnconfiguredFixture() {
        // protojson drops unset optionals — an unconfigured server omits
        // source, accountId and updatedAt.
        val msg = statusAdapter.fromJson(fixture("status_unconfigured.json"))!!
        assertEquals(false, msg.configured)
        assertNull(msg.source)
        assertNull(msg.account_id)
        assertNull(msg.updated_at)
        assertEquals("@cf/openai/whisper-large-v3-turbo", msg.model)
        assertEquals(4194304, msg.max_audio_bytes)
    }

    @Test
    fun absentConfiguredReadsAsNotConfigured() {
        // A response missing `configured` entirely must not be treated as
        // configured.
        val msg = statusAdapter.fromJson(
            "{\"model\":\"@cf/openai/whisper-large-v3-turbo\",\"maxAudioBytes\":4194304}",
        )!!
        assertNull(msg.configured)
    }

    @Test
    fun decodesSetConfigResponse() {
        val body = "{\"status\":${fixture("status_configured.json")},\"removed\":false}"
        val msg = setConfigAdapter.fromJson(body)!!
        assertEquals(false, msg.removed)
        assertEquals(true, msg.status?.configured)
        assertEquals("e32ab932dec1b35e0df9c0ec56ab6c8f", msg.status?.account_id)
    }

    @Test
    fun decodesTranscript() {
        val msg = transcriptAdapter.fromJson(
            "{\"text\":\"hello from audio\",\"model\":\"@cf/openai/whisper-large-v3-turbo\"}",
        )!!
        assertEquals("hello from audio", msg.text)
        assertEquals("@cf/openai/whisper-large-v3-turbo", msg.model)
    }

    @Test
    fun serializesCamelCase() {
        val encoded = statusAdapter.toJson(
            SttStatus(
                configured = true,
                source = "stored",
                account_id = "acct",
                model = "@cf/openai/whisper-large-v3-turbo",
                updated_at = "2026-11-09T10:30:00Z",
                max_audio_bytes = 4194304,
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("configured"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("accountId"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("maxAudioBytes"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("updatedAt"))
    }
}
