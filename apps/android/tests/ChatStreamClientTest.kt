package com.console.mobile.data.stream

import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.ConsoleJson
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

/**
 * Centralized SSE tests (apps/android/tests). Covers the ChatStreamClient
 * against a MockWebServer: frame parsing, id: seq tracking, heartbeat
 * skipping, malformed-frame tolerance, 409 surfacing, and abort.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class ChatStreamClientTest {
    private fun client(server: MockWebServer): ChatStreamClient {
        val http = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.MILLISECONDS)
            .writeTimeout(10, TimeUnit.SECONDS)
            .build()
        return ChatStreamClient(http)
    }

    private fun sseBody(vararg lines: String): String =
        lines.joinToString("\n", postfix = "\n")

    @Test fun parsesFramesWithSeq() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            val body = sseBody(
                "id: 7",
                "event: turnStart",
                "data: {\"type\":\"turnStart\",\"prompt\":\"hi\"}",
                "",
                "id: 8",
                "data: {\"type\":\"streamReset\"}",
                "",
            )
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .addHeader("Content-Type", "text/event-stream")
                    .setBody(body),
            )
            val outcomes = client(server)
                .attach(server.url("/").toString().trimEnd('/'), "s1")
                .toList()
            val frames = outcomes.filterIsInstance<StreamOutcome.Frame>()
            assertEquals(2, frames.size)
            assertEquals(7L, frames[0].frame.seq)
            assertEquals("turnStart", frames[0].frame.event.type)
            assertEquals(8L, frames[1].frame.seq)
            // Terminal end is Completed on clean close.
            assertTrue(outcomes.last() is StreamOutcome.End)
            assertIs<StreamEnd.Completed>((outcomes.last() as StreamOutcome.End).end)
            // Attach URL shape (no since here).
            val req = server.takeRequest(5, TimeUnit.SECONDS)
            assertTrue(req?.path?.contains("/api/sessions/s1/run/stream") == true)
        } finally {
            server.shutdown()
        }
    }

    @Test fun skipsHeartbeatsAndMalformed() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            val body = sseBody(
                ": heartbeat",
                "data: not-json{{{",
                "data: {\"type\":\"sessionEnd\"}",
                "",
            )
            server.enqueue(MockResponse().setResponseCode(200).setBody(body))
            val outcomes = client(server)
                .attach(server.url("/").toString().trimEnd('/'), "s2")
                .toList()
            val frames = outcomes.filterIsInstance<StreamOutcome.Frame>()
            assertEquals(1, frames.size)
            assertEquals("sessionEnd", frames[0].frame.event.type)
            assertNull(frames[0].frame.seq)
        } finally {
            server.shutdown()
        }
    }

    @Test fun surfaces409WithStatusCode() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse().setResponseCode(409)
                    .setBody("{\"success\":false,\"error\":\"No active run\"}"),
            )
            val outcomes = client(server)
                .attach(server.url("/").toString().trimEnd('/'), "gone", since = 3)
                .toList()
            val end = outcomes.filterIsInstance<StreamOutcome.End>().single()
            val failed = assertIs<StreamEnd.Failed>(end.end)
            assertEquals(409, failed.statusCode)
            val req = server.takeRequest(5, TimeUnit.SECONDS)
            assertTrue(req?.path?.contains("since=3") == true)
        } finally {
            server.shutdown()
        }
    }

    @Test fun postsRunBody() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("data: {\"type\":\"done\"}\n\n"))
            val outcomes = client(server)
                .startRun(server.url("/").toString().trimEnd('/'), "abc", "{\"prompt\":\"hi\"}", authToken = "tok")
                .toList()
            assertTrue(outcomes.any { it is StreamOutcome.Frame && it.frame.event.type == "done" })
            val req = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertEquals("POST", req.method)
            assertTrue(req.path?.endsWith("/api/sessions/abc/run") == true)
            assertEquals("Bearer tok", req.getHeader("Authorization"))
        } finally {
            server.shutdown()
        }
    }
}
