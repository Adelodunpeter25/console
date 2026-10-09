package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AuthStatusResponse
import console.v1.GitHubAuthStatus
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the GitHub git-credential status. The fixture lives
 * in proto/testdata/auth and is shared by Go, Rust, and Kotlin.
 */
class GitHubAuthFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val statusAdapter = moshi.adapter(GitHubAuthStatus::class.java)
    private val authStatusAdapter = moshi.adapter(AuthStatusResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/auth/$name").readText().trim()

    @Test
    fun decodesGoldenGitHubStatusFixture() {
        val status = statusAdapter.fromJson(fixture("github_status.json"))!!
        assertTrue(status.connected)
        assertEquals("adelodunpeter", status.username)
        assertEquals(listOf("repo", "workflow"), status.scopes)
    }

    @Test
    fun disconnectedStatusHasNoUsernameOrScopes() {
        val status = statusAdapter.fromJson("""{"connected":false}""")!!
        assertFalse(status.connected)
        assertNull(status.username)
        assertTrue(status.scopes.isEmpty())
    }

    @Test
    fun fineGrainedTokensReportNoScopesAndStillConnect() {
        // Fine-grained PATs return no scope header, so an empty list is normal.
        val status = statusAdapter.fromJson("""{"connected":true,"username":"octocat"}""")!!
        assertTrue(status.connected)
        assertTrue(status.scopes.isEmpty())
    }

    @Test
    fun githubRidesAlongInTheMainAuthStatus() {
        val body = """{"claude":{"logged_in":false},"github":{"connected":true,"username":"octocat"}}"""
        val parsed = authStatusAdapter.fromJson(body)!!
        assertEquals("octocat", parsed.github?.username)
        assertTrue(parsed.github!!.connected)
    }

    @Test
    fun anAbsentGitHubRowReadsAsNull() {
        // The repository maps this to "not connected" instead of crashing.
        val parsed = authStatusAdapter.fromJson("""{"claude":{"logged_in":false}}""")!!
        assertNull(parsed.github)
    }
}
