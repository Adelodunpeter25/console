package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SubagentActivityItem
import console.v1.SubagentInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the subagents migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class SubagentsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<SubagentInfo>>(
        Types.newParameterizedType(List::class.java, SubagentInfo::class.java),
    )

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenSubagentsFixture() {
        val items = listAdapter.fromJson(fixture("subagents.json"))!!
        assertEquals(1, items.size)
        val sub = items[0]
        assertEquals("sub1", sub.subagent_id)
        assertEquals(5, sub.max_turns)
        assertEquals(2, sub.current_turn)
        assertEquals(1700000000000L, sub.created_at)
        assertEquals(1, sub.activities.size)
        val act: SubagentActivityItem = sub.activities[0]
        assertEquals(2, act.turn_index)
        assertEquals("c9", act.tool_call_id)
        assertEquals("{\"path\":\"/x\"}", act.args.toByteArray().toString(Charsets.UTF_8))
    }

    @Test
    fun ignoresUnknownFields() {
        val items = listAdapter.fromJson(
            "[{\"subagentId\":\"s\",\"parentToolCallId\":\"c\",\"name\":\"n\",\"role\":\"r\",\"prompt\":\"p\",\"maxTurns\":1,\"currentTurn\":1,\"status\":\"running\",\"createdAt\":\"1\",\"updatedAt\":\"2\",\"futureField\":1}]",
        )!!
        assertEquals("n", items[0].name)
    }

    @Test
    fun serializesCountsAsNumbers() {
        // int32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
        val encoded = moshi.adapter(SubagentInfo::class.java).toJson(
            SubagentInfo(subagent_id = "s", parent_tool_call_id = "c", name = "n", role = "r", prompt = "p", max_turns = 1, current_turn = 1, status = "running"),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"maxTurns\":1"))
        assertTrue("unexpected encoding: $encoded", !encoded.contains("summary"))
    }
}
