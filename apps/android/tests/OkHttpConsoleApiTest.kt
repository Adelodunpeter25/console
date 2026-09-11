package com.console.mobile.data.api

import com.console.mobile.data.model.SessionHeader
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.builtins.ListSerializer
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

/**
 * Centralized HTTP tests (apps/android/tests): envelope unwrapping,
 * query params, auth header, error mapping, and session endpoints.
 */
class OkHttpConsoleApiTest {
    private fun api(server: MockWebServer, token: String? = "tok"): OkHttpConsoleApi {
        val http = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(10, TimeUnit.SECONDS)
            .build()
        val transport = HttpTransport(
            callClient = http,
            baseUrl = { server.url("/").toString().trimEnd('/') },
            authToken = { token },
        )
        return OkHttpConsoleApi(transport)
    }

    @Test fun listsSessionsWithParamsAndAuth() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"success":true,"data":[{"id":"s1","title":"T","cwd":"/tmp","modelId":"m","provider":"codex","createdAt":1,"updatedAt":2}]}""",
                ),
            )
            val list: List<SessionHeader> = api(server).getSessions("/tmp", null, false)
            assertEquals(1, list.size)
            assertEquals("s1", list[0].id)
            val req = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertTrue(req.path?.contains("/api/sessions") == true)
            assertTrue(req.path?.contains("cwd=") == true)
            assertEquals("Bearer tok", req.getHeader("Authorization"))
        } finally {
            server.shutdown()
        }
    }

    @Test fun failureEnvelopeThrows() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200)
                    .setBody("""{"success":false,"error":"nope"}"""),
            )
            assertFailsWith<ApiException> { api(server).getProviders() }
        } finally {
            server.shutdown()
        }
    }

    @Test fun httpErrorSurfacesServerError() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse().setResponseCode(500)
                    .setBody("""{"success":false,"error":"boom"}"""),
            )
            val err = assertFailsWith<ApiException> { api(server).getSessions() }
            assertTrue(err.message?.contains("boom") == true)
        } finally {
            server.shutdown()
        }
    }

    @Test fun createsSessionWithBody() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse().setResponseCode(200).setBody(
                    """{"success":true,"data":{"id":"n","title":"N","cwd":"/tmp","modelId":"m","provider":"codex","createdAt":1,"updatedAt":1}}""",
                ),
            )
            val header = api(server).createSession(
                com.console.mobile.data.model.CreateSessionDto(cwd = "/tmp"),
            )
            assertEquals("n", header.id)
            val req = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertEquals("POST", req.method)
            assertTrue(req.body.readUtf8().contains("/tmp"))
        } finally {
            server.shutdown()
        }
    }

    @Test fun noTokenMeansNoAuthHeader() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(MockResponse().setResponseCode(200).setBody("""{"success":true,"data":[]}"""))
            api(server, token = null).getSessions()
            val req = server.takeRequest(5, TimeUnit.SECONDS)!!
            assertEquals(null, req.getHeader("Authorization"))
        } finally {
            server.shutdown()
        }
    }

    @Test fun transportUnwrapHelpers() {
        val t = HttpTransport(
            callClient = OkHttpClient(),
            baseUrl = { "http://x" },
            authToken = { null },
        )
        val ok: List<String> = t.unwrap(
            """{"success":true,"data":["a"]}""",
            ListSerializer(kotlinx.serialization.builtins.serializer<String>()),
            "x",
        )
        assertEquals(listOf("a"), ok)
    }
}
