package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SessionFileChange
import console.v1.SessionFileChangeDiff
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the session file changes migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class SessionChangesFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<SessionFileChange>>(
        Types.newParameterizedType(List::class.java, SessionFileChange::class.java),
    )
    private val diffAdapter = moshi.adapter(SessionFileChangeDiff::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenChangesFixture() {
        val items = listAdapter.fromJson(fixture("changes.json"))!!
        assertEquals(1, items.size)
        val change = items[0]
        assertEquals("src/a.ts", change.path)
        assertEquals(3, change.turn_index)
        assertEquals(10, change.additions)
        assertEquals(2, change.deletions)
        assertEquals("--- a\n", change.diff_text)
        assertTrue(change.reviewed)
        assertEquals(1700000000000L, change.updated_at)
    }

    @Test
    fun decodesGoldenDiffFixture() {
        assertEquals("--- a\n", diffAdapter.fromJson(fixture("change_diff.json"))!!.diff_text)
    }

    @Test
    fun ignoresUnknownFields() {
        val items = listAdapter.fromJson(
            "[{\"path\":\"a\",\"turnIndex\":1,\"status\":\"modified\",\"additions\":1,\"deletions\":0,\"reviewed\":false,\"updatedAt\":\"2\",\"futureField\":1}]",
        )!!
        assertEquals("a", items[0].path)
    }

    @Test
    fun serializesCountsAsNumbers() {
        // uint32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
        val encoded = moshi.adapter(SessionFileChange::class.java).toJson(
            SessionFileChange(path = "src/a.ts", turn_index = 3, status = "modified", additions = 10, updated_at = 1700000000000L),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"turnIndex\":3"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"updatedAt\":\"1700000000000\""))
    }
}
