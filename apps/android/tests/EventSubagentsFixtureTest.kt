package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SubagentActivityEvent
import console.v1.SubagentEndEvent
import console.v1.SubagentStartEvent
import console.v1.TodoItem
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the todo update + subagent frame migration (Phase
 * 4, fifth slice). Fixtures live in proto/testdata/event and are shared by
 * Go, Rust, and Kotlin. The subagent payloads stay flattened under the
 * frame's type tag (no nested payload object).
 */
class EventSubagentsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()

    private fun fixture(name: String): String =
        File("../../../proto/testdata/event/$name").readText().trim()

    @Test
    fun decodesGoldenTodoUpdateFrame() {
        val frame = Json.parseToJsonElement(fixture("todo_update.json"))
        val itemsField = frame.let { it as kotlinx.serialization.json.JsonObject }.getValue("items")
        val items = moshi.adapter<List<TodoItem>>(
            com.squareup.moshi.Types.newParameterizedType(List::class.java, TodoItem::class.java),
        ).fromJson(itemsField.toString())!!
        assertEquals(1, items.size)
        assertEquals(1, items[0].id)
        assertEquals("a", items[0].content)
        assertEquals("pending", items[0].status)
    }

    @Test
    fun decodesGoldenSubagentStartFrame() {
        val start = moshi.adapter(SubagentStartEvent::class.java)
            .fromJson(fixture("subagent_start.json"))!!
        assertEquals("s1", start.subagent_id)
        assertEquals("c1", start.parent_tool_call_id)
        assertEquals("w", start.name)
        assertEquals("tester", start.role)
        assertEquals("do it", start.prompt)
        assertEquals(10, start.max_turns)
    }

    @Test
    fun decodesGoldenSubagentActivityFrame() {
        val act = moshi.adapter(SubagentActivityEvent::class.java)
            .fromJson(fixture("subagent_activity.json"))!!
        assertEquals("s1", act.subagent_id)
        assertEquals(2, act.turn_index)
        assertEquals("c2", act.tool_call_id)
        assertEquals("sh", act.tool_name)
        assertEquals("running", act.status)
        assertNull(act.error)
        // Args cross as raw JSON bytes (base64 on the wire).
        assertEquals("{\"cmd\":\"ls\"}", act.args.utf8())
    }

    @Test
    fun decodesGoldenSubagentEndFrame() {
        val end = moshi.adapter(SubagentEndEvent::class.java)
            .fromJson(fixture("subagent_end.json"))!!
        assertEquals("s1", end.subagent_id)
        assertEquals("completed", end.status)
        assertEquals("done", end.summary)
        assertEquals(3, end.total_turns)
    }

    @Test
    fun ignoresUnknownFields() {
        val start = moshi.adapter(SubagentStartEvent::class.java)
            .fromJson("{\"subagentId\":\"s\",\"name\":\"n\",\"role\":\"r\",\"prompt\":\"p\",\"maxTurns\":1,\"futureField\":1}")!!
        assertEquals("s", start.subagent_id)
        assertTrue(start.prompt == "p")
    }
}