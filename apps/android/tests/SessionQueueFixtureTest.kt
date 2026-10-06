package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.QueuedPrompt
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the queued prompts migration. Fixtures live in
 * proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class SessionQueueFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val adapter = moshi.adapter(QueuedPrompt::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenQueueFixture() {
        val msg = adapter.fromJson(fixture("queue.json"))!!
        assertEquals("q1", msg.id)
        assertEquals("s1", msg.session_id)
        assertEquals("fix it", msg.prompt)
        assertEquals(listOf("a.ts"), msg.context_files)
        assertEquals(1, msg.attachments.size)
        assertEquals("aGk=", msg.attachments[0].data_)
        assertEquals("image/png", msg.attachments[0].mime_type)
        assertEquals(1, msg.annotations.size)
        assertEquals("an1", msg.annotations[0].id)
        assertEquals("look", msg.annotations[0].user_comment)
        assertEquals("m", msg.model_id)
        assertEquals("p", msg.provider)
        assertEquals("auto", msg.approval_mode)
        // Server never populates the desktop-optimistic thinking level.
        assertNull(msg.thinking_level)
        // created_at is the server's RFC3339 string, not millis.
        assertEquals("2024-05-01T12:00:00Z", msg.created_at)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = adapter.fromJson(
            "{\"id\":\"q\",\"sessionId\":\"s\",\"prompt\":\"hi\",\"createdAt\":\"2024-05-01T12:00:00Z\",\"futureField\":1}",
        )!!
        assertEquals("q", msg.id)
    }

    @Test
    fun omitsAbsentOptionals() {
        // Optional model/provider/approval stay absent (never empty strings).
        val encoded = adapter.toJson(
            QueuedPrompt(id = "q", session_id = "s", prompt = "hi", created_at = "2024-05-01T12:00:00Z"),
        )
        assertTrue("unexpected encoding: $encoded", !encoded.contains("modelId"))
        assertTrue("unexpected encoding: $encoded", !encoded.contains("thinkingLevel"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"createdAt\":\"2024-05-01T12:00:00Z\""))
    }
}
