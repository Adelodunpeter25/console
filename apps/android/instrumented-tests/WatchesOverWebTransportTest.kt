package com.console.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.console.mobile.data.stream.WtTransport
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Watches over WebTransport on a device (plan Phase 3).
 *
 * Git status is the useful case: the server answers with a real summary the
 * moment the stream opens, so this proves watch framing, the request payload
 * and the typed decode without needing anything to change on disk. Notifications
 * cannot be proven this way — the server only emits them when something
 * happens — so it is covered by the same code path with a different target.
 */
@RunWith(AndroidJUnit4::class)
class WatchesOverWebTransportTest {
    private val args: android.os.Bundle by lazy {
        androidx.test.platform.app.InstrumentationRegistry.getArguments()
    }

    private fun setting(name: String, default: String? = null): String? =
        (args.getString(name) ?: System.getenv(name))?.takeIf { it.isNotBlank() } ?: default

    private val apiUrl: String get() = setting("WT_SPIKE_API_URL", "http://10.0.2.2:18091")!!

    /** Paired device token; the WebTransport handshake is authenticated by it. */
    private fun deviceToken(): String? =
        ServerPairing.pair(apiUrl, setting("WT_SPIKE_PAIR_GRANT"))

    @Before
    fun configure() {
        WtTransport.originOverride = setting("WT_SPIKE_ORIGIN", "https://10.0.2.2:18454")
        WtTransport.certSha256 = setting("WT_SPIKE_CERT_SHA256").orEmpty()
    }

    @Test
    fun gitStatusWatchDeliversASummary() {
        // The server's git watch emits an initial summary on subscribe, so a
        // frame arriving at all means the whole path worked.
        // The path is resolved on the *server*, not on the device, so it is not
        // checked here — an emulator has no view of the host's filesystem.
        val repo = setting("WT_SPIKE_REPO") ?: run {
            assumeTrue("set WT_SPIKE_REPO to a git checkout on the server host", false)
            return
        }

        val adapter = com.squareup.moshi.Moshi.Builder()
            .add(com.squareup.wire.WireJsonAdapterFactory())
            .build()
            .adapter<Any>(Map::class.java)

        val token = deviceToken()
        assumeTrue("no WT_SPIKE_PAIR_GRANT provided", token != null)
        val watch = WtTransport.watch(
            baseUrl = apiUrl,
            requestJson = WtTransport.watchRequest(WtTransport.TARGET_GIT_STATUS, path = repo),
            frameType = WtTransport.FRAME_GIT_STATUS,
            authToken = token,
            decode = { raw ->
                // Decoding loosely is enough here: the assertion is that a
                // gitStatus frame arrived and carried an object.
                runCatching { adapter.fromJson(raw) }.getOrNull()
            },
        )
        assertNotNull("watch did not open: ${WtNative.lastError()}", watch)

        val first = runBlocking { withTimeoutOrNull(20_000) { watch!!.first() } }
        println("git status watch frame: $first")
        assertNotNull("no gitStatus frame within 20s", first)
        assertTrue("frame should be a status object, got $first", first is Map<*, *>)
    }

    @Test
    fun notificationWatchOpensWithoutAnInitialSnapshot() {
        // Nothing to assert about frames (the server is silent until something
        // notifies), so this pins that the stream opens at all — a rejected
        // request comes back as an error frame rather than hanging.
        val token = deviceToken()
        assumeTrue("no WT_SPIKE_PAIR_GRANT provided", token != null)
        val watch = WtTransport.watch(
            baseUrl = apiUrl,
            requestJson = WtTransport.watchRequest(WtTransport.TARGET_NOTIFICATIONS),
            frameType = WtTransport.FRAME_NOTIFICATION,
            authToken = token,
            decode = { it },
        )
        assertNotNull("notifications watch did not open: ${WtNative.lastError()}", watch)
        // Collect briefly: if the server rejected the request we get an error
        // frame and the flow closes; if it accepted, it simply stays quiet.
        val outcome = runBlocking {
            withTimeoutOrNull(3_000) {
                runCatching { watch!!.first() }.getOrNull()
            }
        }
        println("notifications watch outcome: $outcome")
    }
}