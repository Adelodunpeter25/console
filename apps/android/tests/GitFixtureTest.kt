package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.GitBranchesResponse
import console.v1.GitCheckoutResponse
import console.v1.GitDiffResponse
import console.v1.GitStatusSummary
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the git migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/git and are shared by Go, Rust, and Kotlin.
 */
class GitFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val statusAdapter = moshi.adapter(GitStatusSummary::class.java)
    private val branchesAdapter = moshi.adapter(GitBranchesResponse::class.java)
    private val diffAdapter = moshi.adapter(GitDiffResponse::class.java)
    private val checkoutAdapter = moshi.adapter(GitCheckoutResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/git/$name").readText().trim()

    @Test
    fun decodesGoldenStatusFixture() {
        val msg = statusAdapter.fromJson(fixture("status.json"))!!
        assertEquals("main", msg.branch)
        assertFalse(msg.clean)
        assertEquals(2, msg.files.size)
        assertEquals("M", msg.files[0].status)
        assertTrue(msg.files[0].staged)
        assertEquals(12, msg.files[0].additions)
        assertEquals(0, msg.files[1].additions)
    }

    @Test
    fun decodesGoldenBranchesFixture() {
        val msg = branchesAdapter.fromJson(fixture("branches.json"))!!
        assertEquals(2, msg.branches.size)
        assertTrue(msg.branches[0].current)
        assertFalse(msg.branches[1].current)
        assertTrue(msg.is_git_repository)
    }

    @Test
    fun decodesGoldenDiffAndCheckoutFixtures() {
        val diff = diffAdapter.fromJson(fixture("diff.json"))!!
        assertEquals("/r/a.go", diff.path)
        assertTrue(diff.diff.contains("+++ b/a.go"))
        assertEquals("dev", checkoutAdapter.fromJson(fixture("checkout.json"))!!.branch)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = statusAdapter.fromJson(
            "{\"branch\":\"b\",\"clean\":true,\"files\":[],\"futureField\":1}",
        )!!
        assertEquals("b", msg.branch)
    }

    @Test
    fun serializesCountsAsNumbers() {
        // uint32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
        val encoded = statusAdapter.toJson(
            GitStatusSummary(
                branch = "main",
                files = listOf(
                    console.v1.GitFileEntry(path = "/r/a.go", status = "M", additions = 12),
                ),
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"additions\":12"))
        assertTrue("unexpected encoding: $encoded", !encoded.contains("clean"))
    }
}
