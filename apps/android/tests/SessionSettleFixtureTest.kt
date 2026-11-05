package com.console.mobile

import com.console.mobile.core.chat.buildGroupedProjectSections
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ProjectInfo
import console.v1.SessionHeader
import console.v1.SessionSettleResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Settled chats: the wire fixtures and the Home grouping contract. */
class SessionSettleFixtureTest {
    private val moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesGoldenSettleResponse() {
        val adapter = moshi.adapter(SessionSettleResponse::class.java)
        val response = adapter.fromJson(fixture("settle.json"))!!
        assertEquals("s1", response.id)
        assertTrue(response.settled)
    }

    @Test
    fun decodesGoldenSettledHeader() {
        val header = moshi.adapter(SessionHeader::class.java).fromJson(fixture("header_settled.json"))!!
        assertEquals("s1", header.id)
        assertEquals(1700000000005L, header.settled_at)
    }

    private fun header(id: String, settledAt: Long? = null, pinnedAt: Long? = null, projectId: String? = null) =
        SessionHeader(
            id = id,
            title = id,
            cwd = "",
            project_id = projectId,
            model_id = "",
            provider = "",
            created_at = 1_000L,
            updated_at = 1_000L,
            message_count = 1,
            status = "done",
            pinned_at = pinnedAt,
            settled_at = settledAt,
        )

    @Test
    fun settledSessionsLeaveActiveSectionsAndGroupLast() {
        val sessions = listOf(
            header("fresh"),
            header("pinned-one", pinnedAt = 9_000L),
            header("settled-old", settledAt = 5_000L),
            header("settled-new", settledAt = 9_000L),
        )
        val sections = buildGroupedProjectSections(sessions, emptyList<ProjectInfo>(), emptyMap())

        // Settled is its own section at the bottom, below Pinned and General.
        assertEquals(listOf("Pinned", "General", "Settled"), sections.map { it.projectName })
        val settled = sections.last()
        assertEquals("settled", settled.projectId)
        // Most recently settled first.
        assertEquals(listOf("settled-new", "settled-old"), settled.data.map { it.id })
        // Neither settled chat appears in an active section.
        assertTrue(sections.dropLast(1).none { s -> s.data.any { it.settled_at != null && it.settled_at > 0 } })
    }

    @Test
    fun unsettledSessionsHaveNoSettledSection() {
        val sections = buildGroupedProjectSections(listOf(header("only")), emptyList<ProjectInfo>(), emptyMap())
        assertEquals(listOf("General"), sections.map { it.projectName })
    }
}
