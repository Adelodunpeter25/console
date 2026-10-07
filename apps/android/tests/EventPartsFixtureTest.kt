package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ModelStreamPart
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the model stream parts migration (Phase 4, first
 * slice). Fixtures live in proto/testdata/event and are shared by Go, Rust,
 * and Kotlin. Frames stay hand-shaped; only the nested part is schema'd.
 */
class EventPartsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val adapter = moshi.adapter(ModelStreamPart::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/event/$name").readText().trim()

    private fun partOf(name: String): ModelStreamPart.Part {
        val element = kotlinx.serialization.json.Json.parseToJsonElement(fixture(name))
        val raw = element.jsonObject.getValue("part").toString()
        return adapter.fromJson(raw)!!.part!!
    }

    @Test
    fun decodesGoldenTextPart() {
        val part = partOf("stream_part_text.json")
        assertTrue(part is ModelStreamPart.Part.Text)
        assertEquals("hi", (part as ModelStreamPart.Part.Text).value)
    }

    @Test
    fun decodesGoldenThinkingPart() {
        val part = partOf("stream_part_thinking.json")
        assertTrue(part is ModelStreamPart.Part.Thinking)
        assertEquals("hmm", (part as ModelStreamPart.Part.Thinking).value)
    }

    @Test
    fun decodesGoldenToolCallPart() {
        val part = partOf("stream_part_tool_call.json")
        assertTrue(part is ModelStreamPart.Part.ToolCall)
        val preview = (part as ModelStreamPart.Part.ToolCall).value
        assertEquals("c1", preview.id)
        assertEquals("sh", preview.name)
    }

    @Test
    fun ignoresUnknownFields() {
        val part = adapter.fromJson("{\"text\":\"hi\",\"futureField\":1}")!!.part!!
        assertTrue(part is ModelStreamPart.Part.Text)
    }

    @Test
    fun serializesFlatOneof() {
        // Scalars sit directly under part (no wrapper level).
        val encoded = adapter.toJson(ModelStreamPart(part = ModelStreamPart.Part.Text("hi")))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"text\":\"hi\""))
    }
}
