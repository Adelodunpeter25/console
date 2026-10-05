package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.UsageReport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the usage migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/usage and are shared by Go, Rust, and Kotlin.
 */
class UsageFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val reportAdapter = moshi.adapter(UsageReport::class.java)
    private val mapAdapter = moshi.adapter<Map<String, UsageReport?>>(
        Types.newParameterizedType(
            Map::class.java,
            String::class.java,
            UsageReport::class.java,
        ),
    )

    private fun fixture(name: String): String =
        File("../../../proto/testdata/usage/$name").readText().trim()

    @Test
    fun decodesGoldenReportFixture() {
        val msg = reportAdapter.fromJson(fixture("report.json"))!!
        assertEquals("claude", msg.provider)
        assertEquals(1700000000000L, msg.fetched_at)
        assertEquals(1, msg.limits.size)
        val limit = msg.limits[0]
        assertEquals("session", limit.id)
        assertEquals("claude", limit.scope?.provider)
        assertEquals("pro", limit.scope?.tier)
        assertEquals(1700003600000L, limit.window?.resets_at)
        assertEquals(12.5, limit.amount?.used)
        assertEquals("percent", limit.amount?.unit)
        assertEquals("ok", limit.status)
    }

    @Test
    fun decodesGoldenMapFixture() {
        // Explicit nulls decode to null values, matching the logged-out shape.
        val map = mapAdapter.fromJson(fixture("map.json"))!!
        assertEquals(3, map.size)
        assertNull(map["antigravity"])
        assertNull(map["codex"])
        assertEquals("claude", map["claude"]?.provider)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = reportAdapter.fromJson(
            "{\"provider\":\"c\",\"fetchedAt\":\"1\",\"limits\":[],\"metadata\":{\"x\":1},\"raw\":{},\"resetCredits\":{\"availableCount\":2}}",
        )!!
        assertEquals("c", msg.provider)
    }

    @Test
    fun serializesCamelCaseStrings() {
        val encoded = reportAdapter.toJson(
            UsageReport(provider = "c", fetched_at = 1L),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("fetchedAt"))
        assertTrue("unexpected encoding: $encoded", !encoded.contains("limits"))
    }
}
