package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.DeleteProjectResponse
import console.v1.ProjectInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the projects migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/projects and are shared by Go, Rust, and Kotlin.
 *
 * Timestamps arrive as protojson strings. The number-form test below locks
 * in tolerance for pre-migration servers during the rollout.
 */
class ProjectFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val projectAdapter = moshi.adapter(ProjectInfo::class.java)
    private val listAdapter = moshi.adapter<List<ProjectInfo>>(
        Types.newParameterizedType(List::class.java, ProjectInfo::class.java),
    )
    private val deleteAdapter = moshi.adapter(DeleteProjectResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/projects/$name").readText().trim()

    @Test
    fun decodesGoldenProjectFixture() {
        val msg = projectAdapter.fromJson(fixture("project.json"))!!
        assertEquals("p1", msg.id)
        assertEquals("demo", msg.name)
        assertEquals("/tmp/demo", msg.path)
        assertEquals(1700000000000L, msg.created_at)
        assertEquals(1700000000001L, msg.updated_at)
    }

    @Test
    fun decodesGoldenListFixture() {
        val items = listAdapter.fromJson(fixture("list.json"))!!
        assertEquals(1, items.size)
        assertEquals("p1", items[0].id)
    }

    @Test
    fun decodesGoldenDeleteFixture() {
        val msg = deleteAdapter.fromJson(fixture("delete.json"))!!
        assertEquals("p1", msg.id)
        assertTrue(msg.deleted)
    }

    @Test
    fun toleratesLegacyNumberTimestamps() {
        val msg = projectAdapter.fromJson(
            "{\"id\":\"p1\",\"name\":\"demo\",\"path\":\"/tmp/demo\",\"createdAt\":1700000000000,\"updatedAt\":1700000000001}",
        )!!
        assertEquals(1700000000000L, msg.created_at)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = projectAdapter.fromJson(
            "{\"id\":\"a\",\"name\":\"b\",\"path\":\"/c\",\"createdAt\":\"1\",\"updatedAt\":\"2\",\"futureField\":\"x\"}",
        )!!
        assertEquals("a", msg.id)
    }

    @Test
    fun serializesTimestampsAsStrings() {
        val encoded = projectAdapter.toJson(
            ProjectInfo(
                id = "p1",
                name = "demo",
                path = "/tmp/demo",
                created_at = 1700000000000L,
                updated_at = 1700000000001L,
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"createdAt\":\"1700000000000\""))
    }
}
