package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.NotificationEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the notifications migration. Fixtures live in
 * proto/testdata/notification and are shared by Go, Rust, and Kotlin. The
 * notification stream has no envelope: the frame is the payload.
 */
class NotificationFixtureTest {
    private val adapter: com.squareup.moshi.JsonAdapter<NotificationEvent> =
        Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(NotificationEvent::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/notification/$name").readText().trim()

    @Test
    fun decodesGoldenAttentionFixture() {
        val n = adapter.fromJson(fixture("attention.json"))!!
        assertEquals("notification", n.type)
        assertEquals("needs_attention", n.kind)
        assertEquals("s1", n.session_id)
        assertEquals("Needs Attention", n.title)
        assertEquals("Fix the parser", n.subtitle)
        assertEquals("sh is requesting permission", n.body)
    }

    @Test
    fun decodesGoldenDoneFixture() {
        val n = adapter.fromJson(fixture("done.json"))!!
        assertEquals("done", n.kind)
        assertEquals("Done", n.title)
        assertEquals("Fix the parser", n.subtitle)
        assertEquals("all done here", n.body)
    }

    @Test
    fun missingSubtitleDefaultsToBlank() {
        // Absent key (old omitempty) must read as a blank subtitle, which the
        // presenter renders as a single-line banner.
        val n = adapter.fromJson(
            "{\"type\":\"notification\",\"kind\":\"done\",\"sessionId\":\"s\",\"title\":\"Done\",\"body\":\"b\"}",
        )!!
        assertEquals("", n.subtitle)
    }

    @Test
    fun ignoresUnknownFields() {
        val n = adapter.fromJson(
            "{\"type\":\"notification\",\"kind\":\"done\",\"sessionId\":\"s\",\"title\":\"t\",\"body\":\"b\",\"futureField\":1}",
        )!!
        assertEquals("s", n.session_id)
        assertTrue(n.title == "t")
    }
}