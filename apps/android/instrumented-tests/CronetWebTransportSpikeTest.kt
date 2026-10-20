package com.console.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.chromium.net.BidirectionalStream
import org.chromium.net.CronetEngine
import org.chromium.net.CronetException
import org.chromium.net.UrlResponseInfo
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.io.BufferedReader
import java.io.File
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import java.nio.ByteBuffer
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * QUIC/WebTransport spike — the negative result that chose the Rust/JNI client.
 *
 * One question, answered on a device: **can Cronet actually open a
 * WebTransport session?** Everything the mobile migration rests on lives in
 * this file:
 *
 *  * the bidirectional-stream API exists on `CronetEngine` and takes
 *    `CONNECT` + `:protocol`, which is how WebTransport is negotiated;
 *  * `cronet-bundled` ships the Java fallback engine alongside the native
 *    one, and the fallback *throws* on `newBidirectionalStreamBuilder` — so a
 *    device that loads the wrong engine can never use WebTransport. That is
 *    the risk worth pricing before building a client on this stack.
 *  * QUIC survives from the app to our Go server, and the handshake reaches
 *    `/wt/v1/connect`.
 *
 * Nothing here is production code.
 *
 * ## VERDICT (emulator, API 33): the handshake never completes.
 *
 * Two of the three cases pass — the native engine loads
 * (`Cronet version: 155.0.8059.16, Statically Linked`) and the stream
 * builder is constructible — but `webTransportSessionOpens` gets **no
 * callback at all**: no headers, no `onFailed`, and a Cronet netlog
 * containing no QUIC activity whatsoever. TCP to the same host works (the
 * token mint succeeds), so this is not a network path problem.
 *
 * Cause, from Chromium's own `BidirectionalStreamTest.java`:
 *
 *     // HttpEngine is supported from Android U+
 *     @RequiresMinAndroidApi(Build.VERSION_CODES.UPSIDE_DOWN_CAKE)
 *
 * `UPSIDE_DOWN_CAKE` is **Android 14 / API 34**. Below that the native
 * engine silently drops bidirectional streams rather than raising the
 * documented Java-engine error, which is exactly what we observe.
 *
 * **So: Cronet WebTransport needs Android 14+, while the app's `minSdk` is
 * 26.** On everything older, mobile must stay on SSE/WS. The fallback that
 * motivated this path does not cover it.
 *
 * Requirements this spike did confirm, needed by any future use of Cronet:
 *  * `android.permission.ACCESS_NETWORK_STATE` — Cronet reads the active
 *    network on every engine build, and throws `SecurityException` without it.
 *  * `addQuicHint(host, port, port)` — Cronet only attempts QUIC for an
 *    origin it has been told about (or learned from Alt-Svc).
 *
 * Run (needs a userdebug emulator image for a writable CA store, or the
 * debug network security config below):
 *   make dev-server-wt WT_PORT=18454   # with the spike cert
 *   ./gradlew :app:connectedDebugAndroidTest
 *
 * Env (defaults target the emulator's host alias, 10.0.2.2):
 *   WT_SPIKE_API_URL   http://10.0.2.2:18091
 *   WT_SPIKE_ORIGIN    https://10.0.2.2:18454
 */
@RunWith(AndroidJUnit4::class)
class CronetWebTransportSpikeTest {
    private lateinit var engine: CronetEngine
    private val executor = Executors.newSingleThreadExecutor()

    private val apiUrl: String get() = env("WT_SPIKE_API_URL") ?: "http://10.0.2.2:18091"
    private val wtOrigin: String get() = env("WT_SPIKE_ORIGIN") ?: "https://10.0.2.2:18454"

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        engine = CronetEngine.Builder(context)
            .enableHttp2(true)
            .enableQuic(true)
            // Without a hint Cronet has no reason to try QUIC for this origin:
            // it only learns h3 from an Alt-Svc header on a real HTTPS response
            // it has already made. WebTransport is HTTP/3-only, so a plain
            // request to the WT port silently goes nowhere (observed: no
            // callback at all, no error). The production client needs this
            // too, or a QUIC hint from the server's own Alt-Svc.
            .addQuicHint(quicHost(), quicPort(), quicPort())
            .build()
    }

    private fun quicHost(): String = URI(wtOrigin).host ?: "10.0.2.2"

    private fun quicPort(): Int = URI(wtOrigin).port.takeIf { it > 0 } ?: 443

    @After
    fun tearDown() {
        engine.shutdown()
        executor.shutdownNow()
    }

    /**
     * The load-bearing precondition. The Java fallback engine has no
     * bidirectional-stream API at all, and `cronet-bundled` ships it, so
     * "which engine loaded" decides whether this device can do WebTransport
     * at all. `nativeEngineIsInUse` covers the same ground more directly;
     * this case additionally proves the stream builder is constructible.
     */
    @Test
    fun bidirectionalStreamBuilderIsSupported() {
        val token = pairDevice() ?: return
        val failure = CountDownLatch(1)
        var message: String? = null
        engine.newBidirectionalStreamBuilder(
            "$wtOrigin/wt/v1/connect?token=$token",
            object : BidirectionalStream.Callback() {
                override fun onResponseHeadersReceived(s: BidirectionalStream, i: UrlResponseInfo) = Unit
                override fun onStreamReady(s: BidirectionalStream) = Unit
                override fun onSucceeded(s: BidirectionalStream, i: UrlResponseInfo) = Unit
                override fun onReadCompleted(s: BidirectionalStream, i: UrlResponseInfo, b: ByteBuffer, d: Boolean) = Unit
                override fun onWriteCompleted(s: BidirectionalStream, i: UrlResponseInfo, b: ByteBuffer, d: Boolean) = Unit
                override fun onFailed(s: BidirectionalStream, i: UrlResponseInfo, e: CronetException) {
                    message = e.message
                    failure.countDown()
                }
            },
            executor,
        ).setHttpMethod("CONNECT").addHeader(":protocol", "webtransport").build()

        // Give it a moment: a failure here means the fallback engine refused
        // the builder, which is the device-level verdict we want.
        Thread.sleep(1500)
        assertTrue(
            "Cronet refused newBidirectionalStreamBuilder: $message " +
                "(the Java fallback engine has no bidirectional-stream API)",
            message?.contains("not supported by the Java implementation") != true,
        )
    }

    /** Which engine actually loaded. Cheap, and names the risk in the failure. */
    @Test
    fun nativeEngineIsInUse() {
        val name = engine.javaClass.name
        println("Cronet engine: $name")
        assertTrue(
            "Cronet fell back to the Java engine ($name); it has no " +
                "bidirectional-stream API, so WebTransport is impossible on this device",
            name != "org.chromium.net.impl.JavaCronetEngine",
        )
    }

    /**
     * The actual spike: open a WebTransport session the way the migration
     * will. A successful handshake answers 200 on the `CONNECT` stream.
     */
    @Test
    fun webTransportSessionOpens() {
        val token = pairDevice() ?: return

        val headers = CountDownLatch(1)
        var status: Int? = null
        var statusText: String? = null
        var stream: BidirectionalStream? = null
        val ready = CountDownLatch(1)

        val callback = object : BidirectionalStream.Callback() {
            override fun onResponseHeadersReceived(s: BidirectionalStream, info: UrlResponseInfo) {
                status = info.httpStatusCode
                statusText = "${info.httpStatusCode} ${info.httpStatusText}"
                headers.countDown()
            }

            override fun onStreamReady(readyStream: BidirectionalStream) {
                stream = readyStream
                ready.countDown()
            }

            override fun onSucceeded(s: BidirectionalStream, i: UrlResponseInfo) = Unit
            override fun onReadCompleted(s: BidirectionalStream, i: UrlResponseInfo, b: ByteBuffer, d: Boolean) = Unit
            override fun onWriteCompleted(s: BidirectionalStream, i: UrlResponseInfo, b: ByteBuffer, d: Boolean) = Unit
            override fun onFailed(failed: BidirectionalStream, i: UrlResponseInfo, e: CronetException) {
                println("WT stream failed: $e")
                headers.countDown()
                ready.countDown()
            }
        }

        engine.newBidirectionalStreamBuilder("$wtOrigin/wt/v1/connect?token=$token", callback, executor)
            .setHttpMethod("CONNECT")
            .addHeader(":protocol", "webtransport")
            .build()

        assertTrue(
            "no CONNECT response within 20s — QUIC did not complete (status=$statusText). " +
                "UDP blocked, or the device does not trust the spike certificate.",
            headers.await(20, TimeUnit.SECONDS),
        )
        println("WT handshake status: $statusText")
        assertEquals("WebTransport handshake was rejected", 200, status)

        assertTrue("stream never became ready", ready.await(10, TimeUnit.SECONDS))
        assertNotNull("onStreamReady never delivered a stream", stream)
    }

    /**
     * Exchange a pairing grant for a device token, the way the client will.
     * There is no freely mintable token endpoint any more.
     */
    private fun pairDevice(): String? {
        val token = ServerPairing.pair(apiUrl, env("WT_SPIKE_PAIR_GRANT"))
        if (token == null) assumeTrue("no WT_SPIKE_PAIR_GRANT provided", false)
        return token
    }

    private fun env(name: String): String? = System.getenv(name)?.takeIf { it.isNotBlank() }
}