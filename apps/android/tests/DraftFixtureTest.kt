package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.PutDraftResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Golden fixture for the synced chat draft (proto/testdata/drafts). */
class DraftFixtureTest {
    private val adapter = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
        .adapter(PutDraftResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/drafts/$name").readText().trim()

    @Test
    fun decodesGoldenPutResponse() {
        val response = adapter.fromJson(fixture("put_response.json"))!!
        assertTrue(response.applied)
        val draft = response.draft!!
        assertEquals("s1", draft.session_id)
        assertEquals("review @src/a.rs", draft.text)
        assertEquals(1700000000000L, draft.updated_at)
        assertEquals("/repo/src/a.rs", draft.mentions.single().path)
        val comment = draft.comments.single()
        assertEquals(3, comment.start_line)
        assertEquals(5, comment.end_line)
        assertEquals("rename this", comment.text)
    }
}
