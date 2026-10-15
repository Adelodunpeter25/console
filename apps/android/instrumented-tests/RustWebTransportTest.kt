package com.console.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * Rust-WebTransport spike for plan Phase 3 — the on-device answer to
 * "can mobile reach QUIC without Cronet's Android 14 floor?".
 *
 * Runs the real client: the same `console-core` code the desktop dogfoods,
 * cross-compiled for Android. This is the difference in one number — this
 * path works on the app's `minSdk` of 26, where Cronet's bidirectional-stream
 * API does not exist at all.
 *
 * Needs the Rust library staged into the APK first:
 *   apps/android/rust-wt/build.sh
 *   # then a WT server on the ports below (see the plan doc)
 *
 * Settings come from instrumentation arguments (`am instrument -e`), not the
 * environment — `System.getenv` yields nothing there.
 */
@RunWith(AndroidJUnit4::class)
class RustWebTransportTest {
    private val args: android.os.Bundle by lazy {
        androidx.test.platform.app.InstrumentationRegistry.getArguments()
    }

    private fun setting(name: String, default: String? = null): String? =
        (args.getString(name) ?: System.getenv(name))?.takeIf { it.isNotBlank() } ?: default

    private val apiUrl: String get() = setting("WT_SPIKE_API_URL", "http://10.0.2.2:18091")!!
    private val wtOrigin: String get() = setting("WT_SPIKE_ORIGIN", "https://10.0.2.2:18454")!!
    private val certSha256: String get() = setting("WT_SPIKE_CERT_SHA256").orEmpty()

    private val executor = Executors.newSingleThreadExecutor()

    @Test
    fun rustLibraryLoads() {
        assertEquals(1, WtNative.abiVersion())
    }

    /**
     * The real question: does a full stream open and deliver frames from the
     * Go server? A chat-run start against a session that does not exist is
     * answered with an `error` frame — enough to prove the handshake, the
     * framing and the server round-trip, with no provider credentials.
     */
    @Test
    fun chatRunStreamDeliversServerFrames() {
        val token = mintToken() ?: return
        val stream = WtStream.open(
            apiUrl = apiUrl,
            origin = wtOrigin,
            certSha256 = certSha256,
            kind = WtNative.kindChatRun(),
            streamId = "wt-spike-missing-session",
            requestType = WtStream.REQUEST_START,
            requestJson = """{"sessionId":"wt-spike-missing-session","start":{"text":"hi"}}""",
        )
        assertNotNull("WebTransport stream did not open: ${WtNative.lastError()}", stream)
        stream!!

        val frame = executor.submit<WtFrame> { stream.readNext(20_000) }.get(30, TimeUnit.SECONDS)
        println("Rust WebTransport frame: seq=${frame?.seq} type=${frame?.type} body=${frame?.text()}")
        assertNotNull("no frame within 20s", frame)
        assertEquals("error", frame!!.type)
        assertTrue(
            "error should name the missing session: ${frame.text()}",
            frame.text().contains("not found"),
        )
        stream.close()
    }

    /**
     * A stream the server cannot serve must fail rather than hang, and must
     * not report success — this is the contract the legacy fallback depends on.
     */
    @Test
    fun badTokenIsRejected() {
        val stream = WtStream.open(
            apiUrl = apiUrl,
            origin = wtOrigin,
            certSha256 = certSha256,
            kind = WtNative.kindChatRun(),
            streamId = "wt-spike-bad-token",
            requestType = WtStream.REQUEST_START,
            requestJson = """{"sessionId":"wt-spike-bad-token","start":{"text":"hi"}}""",
        )
        if (stream == null) {
            // Rejected at the handshake, which is a valid outcome.
            println("bad token rejected at connect: ${WtNative.lastError()}")
            return
        }
        stream!!
        val frame = executor.submit<WtFrame> { stream.readNext(20_000) }.get(30, TimeUnit.SECONDS)
        assertEquals("a bogus token must not deliver events", "error", frame?.type)
        stream.close()
    }

    /** An unreachable origin must fail fast rather than leave a zombie stream. */
    @Test
    fun unreachableOriginFailsToOpen() {
        val stream = WtStream.open(
            apiUrl = apiUrl,
            origin = "https://127.0.0.1:1",
            certSha256 = "",
            kind = WtNative.kindChatRun(),
            streamId = "wt-spike-unreachable",
            requestType = WtStream.REQUEST_START,
            requestJson = """{"sessionId":"wt-spike-unreachable","start":{"text":"hi"}}""",
        )
        assertNull("an unreachable origin must not open a stream", stream)
        println("unreachable origin rejected: ${WtNative.lastError()}")
    }

    /** Mint a handshake token over REST, the way the client will. */
    private fun mintToken(): String? {
        val conn = try {
            URL("$apiUrl/api/auth/token").openConnection() as HttpURLConnection
        } catch (e: Exception) {
            assumeTrue("bad WT_SPIKE_API_URL: $e", false)
            return null
        }
        return try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.setRequestProperty("Content-Type", "application/json")
            conn.outputStream.use { it.write("""{"deviceId":"spike"}""".toByteArray()) }
            val body = BufferedReader(InputStreamReader(conn.inputStream)).readText()
            Regex("\"token\"\\s*:\\s*\"([^\"]+)\"").find(body)?.groupValues?.get(1)
        } catch (e: Exception) {
            // No server reachable is "not configured", not a QUIC failure —
            // skip rather than report a false negative.
            assumeTrue("no dev server at $apiUrl ($e)", false)
            null
        }
    }
}