package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.DirCreateResponse
import console.v1.FileDeleteResponse
import console.v1.FileSearchResult
import console.v1.FileWriteResponse
import console.v1.FsBrowseResult
import console.v1.FsChangeEvent
import console.v1.FsDirectoryTree
import console.v1.FsFileContent
import console.v1.FsTreeEntry
import console.v1.GrepResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the filesystem migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/fs and are shared by Go, Rust, and Kotlin.
 */
class FsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val browseAdapter = moshi.adapter(FsBrowseResult::class.java)
    private val entryListAdapter = moshi.adapter<List<FsTreeEntry>>(
        Types.newParameterizedType(List::class.java, FsTreeEntry::class.java),
    )
    private val searchListAdapter = moshi.adapter<List<FileSearchResult>>(
        Types.newParameterizedType(List::class.java, FileSearchResult::class.java),
    )
    private val grepAdapter = moshi.adapter(GrepResult::class.java)
    private val treeAdapter = moshi.adapter(FsDirectoryTree::class.java)
    private val fileContentAdapter = moshi.adapter(FsFileContent::class.java)
    private val fileWriteAdapter = moshi.adapter(FileWriteResponse::class.java)
    private val fileDeleteAdapter = moshi.adapter(FileDeleteResponse::class.java)
    private val dirCreateAdapter = moshi.adapter(DirCreateResponse::class.java)
    private val watchAdapter = moshi.adapter(FsChangeEvent::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/fs/$name").readText().trim()

    @Test
    fun decodesGoldenBrowseFixture() {
        val msg = browseAdapter.fromJson(fixture("browse.json"))!!
        assertEquals("/tmp/proj", msg.current_path)
        assertEquals("/tmp", msg.parent_path)
        assertEquals(2, msg.entries.size)
        assertTrue(msg.entries[0].is_dir)
        // Sizes arrive as protojson strings, not numbers.
        assertEquals(1234L, msg.entries[1].size)
    }

    @Test
    fun decodesGoldenEntriesFixture() {
        val items = entryListAdapter.fromJson(fixture("entries.json"))!!
        assertEquals(2, items.size)
        assertEquals(1, items[0].children.size)
        assertEquals("a.go", items[0].children[0].name)
    }

    @Test
    fun decodesGoldenSearchFixture() {
        val items = searchListAdapter.fromJson(fixture("search.json"))!!
        assertEquals(1, items.size)
        assertEquals("src/a.go", items[0].relative_path)
        assertEquals(0.95, items[0].score, 0.0)
    }

    @Test
    fun decodesGoldenGrepFixture() {
        val msg = grepAdapter.fromJson(fixture("grep.json"))!!
        assertEquals(3, msg.total_matched)
        assertEquals(200, msg.next_cursor)
        assertTrue(msg.has_more)
        val m = msg.matches[0]
        assertEquals(12, m.line_number)
        assertEquals(1, m.match_ranges.size)
        assertEquals(4, m.match_ranges[0].start)
    }

    @Test
    fun decodesGoldenSmallFixtures() {
        val tree = treeAdapter.fromJson(fixture("tree.json"))!!
        assertTrue(tree.tree_formatted.contains("a.go"))
        val content = fileContentAdapter.fromJson(fixture("file_content.json"))!!
        assertEquals("package main\n", content.content)
        val wrote = fileWriteAdapter.fromJson(fixture("file_write.json"))!!
        assertTrue(wrote.message.contains("12 bytes"))
        assertTrue(fileDeleteAdapter.fromJson(fixture("file_delete.json"))!!.deleted)
        assertTrue(dirCreateAdapter.fromJson(fixture("dir_create.json"))!!.created)
        val watch = watchAdapter.fromJson(fixture("watch.json"))!!
        assertEquals("/p/a.go", watch.event_path)
    }

    @Test
    fun toleratesLegacyNumberSize() {
        // Pre-migration servers sent sizes as numbers.
        val msg = browseAdapter.fromJson(
            "{\"currentPath\":\"/p\",\"entries\":[{\"name\":\"a\",\"path\":\"/p/a\",\"size\":42}]}",
        )!!
        assertEquals(42L, msg.entries[0].size)
    }

    @Test
    fun ignoresUnknownFields() {
        // Dropped shapes (git status, modified_at, is_binary, metadata, raw)
        // must be ignored, not fatal.
        val msg = entryListAdapter.fromJson(
            "[{\"name\":\"a\",\"path\":\"/p/a\",\"gitStatus\":\"M\",\"modifiedAt\":1,\"isBinary\":false}]",
        )!!
        assertEquals("a", msg[0].name)
    }

    @Test
    fun serializesCamelCase() {
        val encoded = browseAdapter.toJson(
            FsBrowseResult(
                current_path = "/p",
                entries = listOf(
                    FsTreeEntry(name = "a", path = "/p/a", size = 42L),
                ),
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("currentPath"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"size\":\"42\""))
    }
}
