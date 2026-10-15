package com.console.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors

/**
 * Rust-WebTransport spike for plan Phase 3 — the on-device answer to
 * "can mobile reach QUIC without Cronet's Android 14 floor?".
 *
 * Runs the real `wtransport` client (the one the desktop dogfoods) compiled
 * for Android. `roundsToLegacyEquivalent` marks the difference in one number:
 * this path works on the app's `minSdk` of 26, where Cronet's
 * bidirectional-stream API does not exist at all.
 *
 * Needs the Rust library built and staged into the APK first:
 *   apps/android/rust-wt-spike/build.sh
 *   make dev-server-wt WT_PORT=18454        # with the spike certificate
 *
 * Env (defaults target the emulator's host alias, 10.0.2.2):
 *   WT_SPIKE_API_URL   http://10.0.2.2:18091
 *   WT_SPIKE_ORIGIN    https://10.0.2.2:18454
 *   WT_SPIKE_CERT_SHA256  hex SHA-256 of the server's DER certificate
 */
@RunWith(AndroidJUnit4::class)
class RustWebTransportSpikeTest {
    // `am instrument -e key=value` arrives via the instrumentation arguments,
    // not the environment — reading System.getenv() silently yields an empty
    // string, which for the certificate means "fall back to platform roots"
    // and a confusing UnknownIssuer.
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
        // Loading is the first failure mode: a wrong ABI or a missing staging
        // step shows up here rather than as an UnsatisfiedLinkError later.
        assertEquals(1, WtNative.abiVersion())
    }

    @Test
    fun webTransportSessionOpens() {
        val token = mintToken() ?: return
        val code = executor.submit<Int> {
            WtNative.probeSession(wtOrigin, token, certSha256)
        }.get(60, java.util.concurrent.TimeUnit.SECONDS)

        println("Rust WebTransport probe: ${WtNative.describe(code)} (code=$code)")
        println("Rust WebTransport error: ${WtNative.lastError()}")
        assertEquals(
            "Rust WebTransport handshake failed — see the app logcat for the " +
                "reason. If the certificate is self-signed, WT_SPIKE_CERT_SHA256 " +
                "must be set to its SHA-256.",
            WtNative.OK,
            code,
        )
    }

    @Test
    fun badTokenIsRejected() {
        val token = mintToken() ?: return
        val code = executor.submit<Int> {
            WtNative.probeSession(wtOrigin, "ct1_bogus.bogus", certSha256)
        }.get(60, java.util.concurrent.TimeUnit.SECONDS)
        // The server rejects an invalid handshake token, so this must not
        // report success. Confirms the result code is meaningful in both
        // directions rather than always zero.
        assertTrue(
            "a bogus token must not report success (got ${WtNative.describe(code)})",
            code != WtNative.OK,
        )
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
            assumeTrue("no dev server at $apiUrl ($e)", false)
            null
        }
    }

}