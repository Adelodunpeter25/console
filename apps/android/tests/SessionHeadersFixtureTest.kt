package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SessionDeleteResponse
import console.v1.SessionHeader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the session headers migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class SessionHeadersFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val headerAdapter = moshi.adapter(SessionHeader::class.java)
    private val listAdapter = moshi.adapter<List<SessionHeader>>(
        Types.newParameterizedType(List::class.java, SessionHeader::class.java),
    )
    private val deleteAdapter = moshi.adapter(SessionDeleteResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenHeaderFixture() {
        val msg = headerAdapter.fromJson(fixture("header.json"))!!
        assertEquals("s1", msg.id)
        assertEquals("gpt-5", msg.model_id)
        assertEquals("p1", msg.project_id)
        assertEquals("medium", msg.thinking_level)
        // Timestamps arrive as protojson strings, not numbers.
        assertEquals(1700000000000L, msg.created_at)
        assertEquals(1700000000001L, msg.updated_at)
        assertEquals(3, msg.message_count)
        assertEquals("done", msg.status)
        assertEquals("feat", msg.worktree?.branch)
    }

    @Test
    fun decodesGoldenListFixture() {
        val items = listAdapter.fromJson(fixture("list.json"))!!
        assertEquals(1, items.size)
        assertEquals("s1", items[0].id)
    }

    @Test
    fun decodesGoldenDeleteFixtures() {
        val deleted = deleteAdapter.fromJson(fixture("delete.json"))!!
        assertEquals("s1", deleted.id)
        assertTrue(deleted.deleted)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = headerAdapter.fromJson(
            "{\"id\":\"s\",\"title\":\"t\",\"cwd\":\"/\",\"modelId\":\"m\",\"provider\":\"p\",\"approvalMode\":\"a\",\"createdAt\":\"1\",\"updatedAt\":\"2\",\"status\":\"done\",\"futureField\":1}",
        )!!
        assertEquals("s", msg.id)
        assertNull(msg.project_id)
    }

    @Test
    fun serializesCamelCaseStrings() {
        val encoded = headerAdapter.toJson(
            SessionHeader(id = "s1", title = "t", cwd = "/", model_id = "m", provider = "p", approval_mode = "a", created_at = 1L, updated_at = 2L, status = "working"),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("createdAt"))
        assertTrue("unexpected encoding: $encoded", !encoded.contains("messageCount"))
    }
}
