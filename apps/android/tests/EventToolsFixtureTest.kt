package com.console.mobile

import com.console.mobile.data.model.toUi
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ToolCall
import console.v1.ToolResult
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the tool execution frames migration (Phase 4,
 * second slice). Fixtures live in proto/testdata/event and are shared by Go,
 * Rust, and Kotlin. Frames stay hand-shaped; call/result payloads are the
 * transcript schema (args/content as JSON bytes).
 */
class EventToolsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val callAdapter = moshi.adapter(ToolCall::class.java)
    private val resultAdapter = moshi.adapter(ToolResult::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/event/$name").readText().trim()

    private fun frame(name: String): kotlinx.serialization.json.JsonObject =
        kotlinx.serialization.json.Json.parseToJsonElement(fixture(name)).jsonObject

    @Test
    fun decodesGoldenStartFrame() {
        val calls = frame("tool_execution_start.json").getValue("calls").jsonArray
        assertEquals(2, calls.size)
        val first = callAdapter.fromJson(calls[0].toString())!!
        assertEquals("c1", first.id)
        assertEquals("sh", first.name)
        assertEquals("sig", first.thought_signature)
        // Render model recovers the JSON args from the bytes.
        assertEquals("ls", first.toUi().arguments?.jsonObject?.getValue("cmd")?.toString()?.trim('"'))
        val bare = callAdapter.fromJson(calls[1].toString())!!
        assertEquals("c2", bare.id)
        assertTrue(bare.arguments.utf8().isEmpty())
        assertNull(bare.thought_signature)
    }

    @Test
    fun decodesGoldenResultFrame() {
        val raw = frame("tool_execution_result.json").getValue("result").toString()
        val result = resultAdapter.fromJson(raw)!!
        assertEquals("c1", result.tool_call_id)
        assertEquals("sh", result.tool_name)
        assertEquals("\"ok\"", result.content.utf8())
        assertNull(result.is_error)
        assertEquals("ok", result.toUi().content?.toString()?.trim('"'))
    }

    @Test
    fun decodesGoldenEndFrame() {
        val results = frame("tool_execution_end.json").getValue("results").jsonArray
        assertEquals(2, results.size)
        val second = resultAdapter.fromJson(results[1].toString())!!
        assertEquals(true, second.is_error)
    }

    @Test
    fun ignoresUnknownFields() {
        val call = callAdapter.fromJson("{\"id\":\"c\",\"name\":\"n\",\"futureField\":1}")!!
        assertEquals("c", call.id)
    }
}
