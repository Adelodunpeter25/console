package com.console.mobile.data.stream

import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader

/**
 * SSE frame emitted by the server. Mirrors the NativeStreamModule Kotlin
 * implementation in apps/mobile/modules/native-stream: `id:` lines feed
 * `seq` (resume cursor for `?since=`), `data:` lines carry AgentSessionEvent
 * JSON. Heartbeat comments (`:`) and blank lines are skipped.
 */
data class SseStreamFrame(val seq: Long?, val event: AgentSessionEvent)

/** Terminal state for a stream — mirrors onStreamEnd/onStreamError contract. */
sealed interface StreamEnd {
    data object Completed : StreamEnd
    data object Aborted : StreamEnd
    data class Failed(val message: String, val statusCode: Int? = null) : StreamEnd
}

class ChatStreamClient(
    private val httpClient: OkHttpClient,
    private val json: Json = ConsoleJson,
) {
    fun startRun(
        baseUrl: String,
        sessionId: String,
        bodyJson: String,
        authToken: String? = null,
    ): Flow<StreamOutcome> = stream(
        url = "${baseUrl.trimEnd('/')}/api/sessions/$sessionId/run",
        method = Method.Post(bodyJson),
        authToken = authToken,
    )

    fun attach(
        baseUrl: String,
        sessionId: String,
        since: Long? = null,
        authToken: String? = null,
    ): Flow<StreamOutcome> {
        val base = "${baseUrl.trimEnd('/')}/api/sessions/$sessionId/run/stream"
        val url = if (since != null) "$base?since=$since" else base
        return stream(url = url, method = Method.Get, authToken = authToken)
    }

    private sealed interface Method {
        data class Post(val bodyJson: String) : Method
        data object Get : Method
    }

    private fun stream(url: String, method: Method, authToken: String?): Flow<StreamOutcome> = callbackFlow {
        val builder = Request.Builder()
            .url(url)
            .addHeader("Accept", "text/event-stream")
            .addHeader("Cache-Control", "no-cache")
        if (authToken != null) builder.addHeader("Authorization", "Bearer $authToken")
        when (method) {
            is Method.Post -> builder.post(method.bodyJson.toRequestBody("application/json; charset=utf-8".toMediaType()))
            is Method.Get -> builder.get()
        }
        val call = httpClient.newCall(builder.build())

        val job = launch(Dispatchers.IO) {
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    if (call.isCanceled()) {
                        trySend(StreamOutcome.End(StreamEnd.Aborted))
                    } else {
                        trySend(StreamOutcome.End(StreamEnd.Failed(e.message ?: "Network error")))
                    }
                    close()
                }

                override fun onResponse(call: Call, response: Response) {
                    if (!response.isSuccessful) {
                        val body = try { response.body?.string().orEmpty() } catch (_: Exception) { "" }
                        response.close()
                        val msg = if (body.isNotBlank()) "Server returned HTTP ${response.code}: $body"
                        else "Server returned HTTP ${response.code}"
                        trySend(StreamOutcome.End(StreamEnd.Failed(msg, response.code)))
                        close()
                        return
                    }
                    val responseBody = response.body
                    if (responseBody == null) {
                        response.close()
                        trySend(StreamOutcome.End(StreamEnd.Failed("Empty response body")))
                        close()
                        return
                    }
                    try {
                        val reader = BufferedReader(InputStreamReader(responseBody.byteStream(), Charsets.UTF_8))
                        var line: String?
                        var aborted = false
                        var lastId: Long? = null
                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) {
                                aborted = true
                                break
                            }
                            val current = line?.trim() ?: continue
                            if (current.isEmpty()) continue
                            if (current.startsWith(":")) continue // heartbeat comment
                            if (current.startsWith("event:")) continue // type is inside data JSON
                            if (current.startsWith("id:")) {
                                lastId = current.substring(3).trim().toLongOrNull()
                                continue
                            }
                            if (!current.startsWith("data:")) continue
                            val raw = current.substring(5).trim()
                            if (raw.isEmpty()) continue
                            try {
                                val event = json.decodeFromString<AgentSessionEvent>(raw)
                                // Fall back to seq from inline id when the event has none.
                                trySend(StreamOutcome.Frame(SseStreamFrame(lastId, event)))
                            } catch (_: Exception) {
                                // ignore malformed frame (parity with TS/bridge behavior)
                            }
                        }
                        trySend(StreamOutcome.End(if (aborted) StreamEnd.Aborted else StreamEnd.Completed))
                    } catch (e: Exception) {
                        if (!call.isCanceled()) {
                            trySend(StreamOutcome.End(StreamEnd.Failed(e.message ?: "Stream reading error")))
                        } else {
                            trySend(StreamOutcome.End(StreamEnd.Aborted))
                        }
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

sealed interface StreamOutcome {
    data class Frame(val frame: SseStreamFrame) : StreamOutcome
    data class End(val end: StreamEnd) : StreamOutcome
}

/** Extract HTTP status from an error envelope body when present. */
fun parseErrorStatus(body: String): Int? = try {
    ConsoleJson.parseToJsonElement(body).jsonObject["status"]?.jsonPrimitive?.intOrNull
} catch (_: Exception) {
    null
}
