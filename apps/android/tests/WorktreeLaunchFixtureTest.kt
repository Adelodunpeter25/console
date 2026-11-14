package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SessionHeader
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture coverage for the instant worktree launch fields
 * (launchState/launchError on SessionHeader, POST /api/sessions/with-worktree).
 * Fixtures live in proto/testdata/session and are shared by Go, Rust, and Kotlin.
 */
class WorktreeLaunchFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val headerAdapter = moshi.adapter(SessionHeader::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/session/$name").readText().trim()

    @Test
    fun decodesProvisioningHeader() {
        val msg = headerAdapter.fromJson(fixture("header_launch.json"))!!
        assertEquals("provisioning", msg.launch_state)
        assertNull(msg.launch_error)
    }

    @Test
    fun decodesFailedHeader() {
        val msg = headerAdapter.fromJson(
            """{"id":"s","title":"t","cwd":"/","approvalMode":"a","createdAt":"1","updatedAt":"2","status":"idle","launchState":"failed","launchError":"unknown base branch"}""",
        )!!
        assertEquals("failed", msg.launch_state)
        assertEquals("unknown base branch", msg.launch_error)
    }

    @Test
    fun plainHeaderHasNoLaunchState() {
        val msg = headerAdapter.fromJson(fixture("header.json"))!!
        assertNull(msg.launch_state)
        assertNull(msg.launch_error)
    }

    @Test
    fun serializesCamelCaseLaunchFields() {
        val encoded = headerAdapter.toJson(
            SessionHeader(
                id = "s1", title = "t", cwd = "/", approval_mode = "a",
                created_at = 1L, updated_at = 2L, status = "idle",
                launch_state = "failed", launch_error = "boom",
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"launchState\":\"failed\""))
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"launchError\":\"boom\""))
    }
}
