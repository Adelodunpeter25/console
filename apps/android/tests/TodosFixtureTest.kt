package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.TodoItem
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the todos migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class TodosFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<TodoItem>>(
        Types.newParameterizedType(List::class.java, TodoItem::class.java),
    )
    private val itemAdapter = moshi.adapter(TodoItem::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenTodosFixture() {
        val items = listAdapter.fromJson(fixture("todos.json"))!!
        assertEquals(2, items.size)
        assertEquals(1, items[0].id)
        assertEquals("Write code", items[0].content)
        assertEquals("in_progress", items[0].status)
        assertEquals("pending", items[1].status)
    }

    @Test
    fun ignoresUnknownFields() {
        val items = listAdapter.fromJson(
            "[{\"id\":1,\"content\":\"x\",\"status\":\"pending\",\"futureField\":1}]",
        )!!
        assertEquals(1, items[0].id)
    }

    @Test
    fun serializesIdsAsNumbers() {
        // int32 ids stay JSON numbers (unlike int64 timestamps elsewhere).
        val encoded = itemAdapter.toJson(TodoItem(id = 1, content = "Write code", status = "in_progress"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"id\":1"))
    }
}
