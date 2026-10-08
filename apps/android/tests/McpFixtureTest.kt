package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.McpServerConfig
import console.v1.McpServerDetail
import console.v1.McpServerStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the MCP servers migration. Fixtures live in
 * proto/testdata/mcp and are shared by Go, Rust, and Kotlin. The list rows
 * are flat (config keys plus live status); the detail nests the config.
 */
class McpFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<McpServerStatus>>(
        Types.newParameterizedType(List::class.java, McpServerStatus::class.java),
    )

    private fun fixture(name: String): String =
        File("../../../proto/testdata/mcp/$name").readText().trim()

    @Test
    fun decodesGoldenServersFixture() {
        val rows = listAdapter.fromJson(fixture("servers.json"))!!
        assertEquals(2, rows.size)
        val full = rows[0]
        assertEquals("atlassian", full.id)
        assertEquals("Atlassian", full.label)
        assertEquals("http", full.transport)
        assertEquals("https://mcp.example", full.url)
        assertEquals("eu", full.env["REGION"])
        assertEquals("static", full.auth?.type)
        assertEquals("atlassian", full.auth?.token_ref)
        assertEquals("write", full.tier_overrides["read"])
        assertEquals(true, full.enabled)
        // int64 timestamps arrive as protojson strings (unread client-side).
        assertEquals(1700000000000L, full.created_at)
        assertEquals("connected", full.status)
        assertEquals(2, full.tool_count)
        assertEquals(2, full.tools.size)
        assertEquals("t1", full.tools[0].name)
        assertEquals("d1", full.tools[0].description)
        assertNull(full.tools[1].description)

        // Minimal stdio row: absent keys decode to defaults, explicit
        // enabled:false stays present (never confused with absent).
        val minimal = rows[1]
        assertEquals("local", minimal.id)
        assertEquals("", minimal.label)
        assertEquals("stdio", minimal.transport)
        assertEquals("npx", minimal.command)
        assertEquals(listOf("-y", "pkg"), minimal.args)
        assertEquals(false, minimal.enabled)
        assertEquals("disconnected", minimal.status)
        assertTrue(minimal.tools.isEmpty())
    }

    @Test
    fun decodesGoldenDetailFixture() {
        val raw = kotlinx.serialization.json.Json.parseToJsonElement(fixture("server_detail.json"))
            .let { it as kotlinx.serialization.json.JsonObject }
        val config = moshi.adapter(McpServerConfig::class.java)
            .fromJson(raw.getValue("config").toString())!!
        assertEquals("atlassian", config.id)
        assertEquals(true, config.enabled)
        assertEquals("https://mcp.example", config.url)
    }

    @Test
    fun ignoresUnknownFields() {
        val row = moshi.adapter(McpServerStatus::class.java).fromJson(
            "{\"id\":\"s\",\"label\":\"S\",\"transport\":\"stdio\",\"enabled\":true,\"status\":\"disconnected\",\"futureField\":1}",
        )!!
        assertEquals("s", row.id)
        assertTrue(row.enabled!!)
    }

    @Test
    fun rowSerializesTimestampsAsStrings() {
        // int64 fields encode as protojson strings; counts stay numbers.
        val encoded = moshi.adapter(McpServerStatus::class.java).toJson(
            McpServerStatus(id = "s", transport = "stdio", enabled = true,
                created_at = 1700000000000L, status = "disconnected", tool_count = 2),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"createdAt\":\"1700000000000\""))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"toolCount\":2"))
    }
}