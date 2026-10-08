package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AssistFileSearchResponse
import console.v1.SlashCommandInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the assist migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/assist and are shared by Go, Rust, and Kotlin.
 */
class AssistFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<SlashCommandInfo>>(
        Types.newParameterizedType(List::class.java, SlashCommandInfo::class.java),
    )
    private val searchAdapter = moshi.adapter(AssistFileSearchResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/assist/$name").readText().trim()

    @Test
    fun decodesGoldenCommandsFixture() {
        val cmds = listAdapter.fromJson(fixture("commands.json"))!!
        assertEquals(3, cmds.size)
        assertEquals("init", cmds[0].name)
        assertTrue(cmds[0].builtin)
        assertEquals("computer-use", cmds[1].name)
        assertTrue(cmds[1].builtin)
        // A discovered skill omits `builtin` on the wire and reads as false.
        assertEquals("find-skills", cmds[2].name)
        assertFalse(cmds[2].builtin)
    }

    @Test
    fun decodesGoldenSearchFixture() {
        val msg = searchAdapter.fromJson(fixture("search.json"))!!
        assertEquals("/p", msg.root)
        assertEquals("DiffView", msg.query)
        assertEquals(2, msg.items.size)
        assertEquals("src/DiffView.kt", msg.items[0].relative_path)
        assertEquals("/p/src/DiffView.kt", msg.items[0].absolute_path)
        assertFalse(msg.items[0].is_dir)
        assertEquals(0.95, msg.items[0].score, 1e-9)
        assertTrue(msg.items[1].is_dir)
    }

    @Test
    fun decodesEmptyQueryFixture() {
        // protojson omits `query` for a bare "@"; the client still reads "".
        val msg = searchAdapter.fromJson(fixture("search_empty_query.json"))!!
        assertEquals("", msg.query)
        assertEquals(1, msg.items.size)
        assertEquals("AGENTS.md", msg.items[0].relative_path)
        assertEquals(0.0, msg.items[0].score, 1e-9)
    }

    @Test
    fun ignoresUnknownFields() {
        val cmds = listAdapter.fromJson(
            """[{"name":"a","description":"b","builtin":true,"futureField":"x"}]""",
        )!!
        assertEquals("a", cmds[0].name)
        assertTrue(cmds[0].builtin)
    }
}