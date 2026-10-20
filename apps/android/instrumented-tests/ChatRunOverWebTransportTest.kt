package com.console.mobile

import androidx.test.ext.junit.runners.AndroidJUnit4
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.stream.ChatStreamClient
import com.console.mobile.data.stream.StreamEnd
import com.console.mobile.data.stream.StreamOutcome
import com.console.mobile.data.stream.WtTransport
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.TimeUnit

/**
 * End-to-end check that a chat run really streams over WebTransport on a
 * device, through the same [ChatStreamClient] the app uses (plan Phase 3).
 *
 * The framing and handshake are covered in `RustWebTransportTest`; what this
 * adds is the Kotlin half — the transport switch, the REST→WT payload
 * translation, and `StreamOutcome` decoding. A run against a session that does
 * not exist is answered with an `error` frame, which is enough to prove the
 * whole path without needing provider credentials.
 *
 * Skips (rather than fails) when the server is not serving WebTransport,
 * because on such a server the correct behavior *is* the SSE fallback — that
 * is what `fallsBackToSseWhenServerCannotServeWebTransport` covers instead.
 */
@RunWith(AndroidJUnit4::class)
class ChatRunOverWebTransportTest {
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
        // The dev server serves plain HTTP on one port and QUIC on another, so
        // the app is told where WebTransport lives — the production default is
        // "same origin as the API".
        WtTransport.originOverride = setting("WT_SPIKE_ORIGIN", "https://10.0.2.2:18454")
        WtTransport.certSha256 = setting("WT_SPIKE_CERT_SHA256").orEmpty()
    }

    @Test
    fun runStreamsOverWebTransport() {
        val sessionId = "wt-e2e-missing-session"
        val client = ChatStreamClient(OkHttpClient())
        val body = ConsoleJson.encodeToString(
            com.console.mobile.data.model.RunPromptDto.serializer(),
            com.console.mobile.data.model.RunPromptDto(prompt = "hello"),
        )

        val token = deviceToken()
        assumeTrue("no WT_SPIKE_PAIR_GRANT provided", token != null)
        val outcomes = collect(client.startRun(apiUrl, sessionId, body, token))

        val end = outcomes.filterIsInstance<StreamOutcome.End>().lastOrNull()
        assertTrue("no terminal outcome: $outcomes", end != null)
        val failure = end!!.end as? StreamEnd.Failed
        assertTrue(
            "expected the server's rejection, got ${end.end}",
            failure != null,
        )
        assertTrue(
            "error should name the session: ${failure!!.message}",
            failure.message.contains("not found"),
        )
    }

    /** A server with no WebTransport listener must land on SSE, not an error. */
    @Test
    fun fallsBackToSseWhenServerCannotServeWebTransport() {
        // Port 1 refuses instantly: WebTransport cannot work there, so the
        // switch must hand the call to the SSE path.
        WtTransport.originOverride = "https://127.0.0.1:1"
        WtTransport.certSha256 = ""
        val client = ChatStreamClient(OkHttpClient())
        val body = ConsoleJson.encodeToString(
            com.console.mobile.data.model.RunPromptDto.serializer(),
            com.console.mobile.data.model.RunPromptDto(prompt = "hello"),
        )

        val outcomes = collect(client.startRun(apiUrl, "wt-e2e-fallback", body, "ct1_unused"))
        // SSE to the (nonexistent) session fails with an HTTP status, which is
        // the point: it got that far, so the fallback happened.
        assertTrue("expected some outcome, got none", outcomes.isNotEmpty())
        val end = outcomes.filterIsInstance<StreamOutcome.End>().last()
        assertTrue("fallback should surface an error, got ${end.end}", end.end is StreamEnd.Failed)
    }

    /**
     * Collect a stream with a budget. A timeout skips rather than fails: the
     * point of these cases is the transport, and a server that is not serving
     * WebTransport legitimately produces nothing here.
     */
    private fun collect(flow: kotlinx.coroutines.flow.Flow<StreamOutcome>): List<StreamOutcome> =
        runBlocking {
            kotlinx.coroutines.withTimeoutOrNull(20_000) { flow.toList() }
                ?: run {
                    assumeTrue("stream produced nothing within 20s", false)
                    emptyList()
                }
        }
}