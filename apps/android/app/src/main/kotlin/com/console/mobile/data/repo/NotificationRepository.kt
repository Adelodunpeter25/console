package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.NotificationEvent
import java.io.BufferedReader
import java.io.InputStreamReader
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import java.io.IOException

class NotificationRepository(
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
) {
    /**
     * Subscribes to backend notifications via GET /api/notifications/stream SSE.
     */
    fun notifications(): Flow<NotificationEvent> = callbackFlow {
        val baseUrl = apiClient.baseUrl.trimEnd('/')
        val url = "$baseUrl/api/notifications/stream"

        val reqBuilder = Request.Builder()
            .url(url)
            .addHeader("Accept", "text/event-stream")
            .addHeader("Cache-Control", "no-cache")

        apiClient.authToken?.let {
            reqBuilder.addHeader("Authorization", "Bearer $it")
        }

        val call = httpClient.newCall(reqBuilder.build())

        val job = launch(Dispatchers.IO) {
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    close()
                }

                override fun onResponse(call: Call, response: Response) {
                    if (!response.isSuccessful) {
                        response.close()
                        close()
                        return
                    }
                    val body = response.body
                    if (body == null) {
                        response.close()
                        close()
                        return
                    }
                    try {
                        val reader = BufferedReader(InputStreamReader(body.byteStream(), Charsets.UTF_8))
                        var line: String?
                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) break
                            val l = line?.trim() ?: continue
                            if (l.isEmpty() || l.startsWith(":") || l.startsWith("event:")) continue
                            if (l.startsWith("data:")) {
                                val raw = l.substring(5).trim()
                                if (raw.isNotEmpty()) {
                                    try {
                                        val notif = ConsoleJson.decodeFromString(NotificationEvent.serializer(), raw)
                                        trySend(notif)
                                    } catch (_: Exception) {
                                    }
                                }
                            }
                        }
                    } catch (_: Exception) {
                    } finally {
                        response.close()
                        close()
                    }
                }
            })
        }

        awaitClose {
            job.cancel()
            call.cancel()
        }
    }
}
