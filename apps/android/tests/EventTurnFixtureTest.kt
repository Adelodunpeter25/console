package com.console.mobile

import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.toUi
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentAssistantMessage
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the turn close + context migration (Phase 4,
 * third slice). Fixtures live in proto/testdata/event and are shared by Go,
 * Rust, and Kotlin. Frames stay hand-shaped; the turn reuses the transcript
 * schema and context numbers stay JSON numbers.
 */
class EventTurnFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val turnAdapter = moshi.adapter(AgentAssistantMessage::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/event/$name").readText().trim()

    private fun frame(name: String): kotlinx.serialization.json.JsonObject =
        kotlinx.serialization.json.Json.parseToJsonElement(fixture(name)).jsonObject

    @Test
    fun decodesGoldenTurnFrame() {
        val turn = turnAdapter.fromJson(frame("model_stream_end.json").getValue("turn").toString())!!
        assertEquals("a1", turn.id)
        assertEquals("end_turn", turn.stop_reason)
        assertEquals(2, turn.content.size)
        // Render model recovers text and tool args.
        val ui = turn.toUi()
        assertEquals("a1", ui.id)
        assertEquals(2, ui.content.size)
        val toolCall = ui.content.filterIsInstance<ToolCallPart>().single()
        assertEquals("c1", toolCall.call.id)
        assertTrue(toolCall.call.arguments.toString().contains("ls"))
    }

    @Test
    fun decodesGoldenContextFrame() {
        // Numbers stay numbers: the hand shape needs no migration.
        val context = frame("context_update.json").getValue("context").jsonObject
        assertEquals(25000, context.getValue("usedTokens").toString().toInt())
        assertEquals(200000, context.getValue("contextWindow").toString().toInt())
        assertEquals("live", context.getValue("source").toString().trim('"'))
    }

    @Test
    fun ignoresUnknownFields() {
        val turn = turnAdapter.fromJson("{\"id\":\"a\",\"content\":[],\"stopReason\":\"\",\"futureField\":1}")!!
        assertEquals("a", turn.id)
    }
}
