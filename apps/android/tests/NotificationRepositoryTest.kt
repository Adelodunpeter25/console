package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.model.NotificationEvent
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

@OptIn(ExperimentalCoroutinesApi::class)
class NotificationRepositoryTest {
    @Test fun testNotificationSseStream() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            val sseBody = "data: {\"type\":\"notification\",\"kind\":\"done\",\"sessionId\":\"s123\",\"title\":\"Agent done\",\"body\":\"All tasks completed\"}\n\n"
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .addHeader("Content-Type", "text/event-stream")
                    .setBody(sseBody),
            )
            val http = OkHttpClient.Builder().connectTimeout(5, TimeUnit.SECONDS).build()
            val client = ConsoleApiClient(http, http, baseUrlOverride = server.url("/").toString().trimEnd('/'))
            val repo = NotificationRepository(client, http)

            val event = repo.notifications().first()
            assertEquals("notification", event.type)
            assertEquals("done", event.kind)
            assertEquals("s123", event.sessionId)
            assertEquals("Agent done", event.title)
            assertEquals("All tasks completed", event.body)
        } finally {
            server.shutdown()
        }
    }
}
