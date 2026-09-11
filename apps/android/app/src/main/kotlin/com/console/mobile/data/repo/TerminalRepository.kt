package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.INPUT_FRAME_TAG
import com.console.mobile.data.model.OUTPUT_FRAME_TAG
import com.console.mobile.data.model.TerminalClientMessage
import com.console.mobile.data.model.TerminalErrorEvent
import com.console.mobile.data.model.TerminalExitEvent
import com.console.mobile.data.model.TerminalKillMessage
import com.console.mobile.data.model.TerminalOutputEvent
import com.console.mobile.data.model.TerminalRecord
import com.console.mobile.data.model.TerminalResizeMessage
import com.console.mobile.data.model.TerminalServerMessage
import com.console.mobile.data.model.TerminalSpawnParams
import com.console.mobile.data.model.TerminalSpawnedEvent
import com.console.mobile.data.model.TerminalStatus
import com.console.mobile.data.model.buildTerminalWsUrl
import com.console.mobile.data.store.TerminalStateHolder
import java.nio.charset.StandardCharsets
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString

interface TerminalSink {
    fun input(data: String)
    fun resize(cols: Int, rows: Int)
    fun kill()
    fun close()
}

class TerminalRepository(
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
    private val terminalState: TerminalStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    private val sinks = ConcurrentHashMap<String, TerminalSink>()
    private val openingDeferreds = ConcurrentHashMap<String, CompletableDeferred<TerminalSpawnedEvent>>()

    suspend fun openTerminal(
        projectId: String,
        cwd: String,
        cols: Int = 80,
        rows: Int = 24,
        label: String? = null,
        shell: String? = null,
    ): TerminalSpawnedEvent {
        val cacheKey = "$projectId::$cwd"
        val existing = openingDeferreds[cacheKey]
        if (existing != null) return existing.await()

        val deferred = CompletableDeferred<TerminalSpawnedEvent>()
        openingDeferreds[cacheKey] = deferred

        val params = TerminalSpawnParams(
            cwd = cwd,
            cols = cols,
            rows = rows,
            label = label,
            shell = shell,
            proto = "binary",
        )
        val url = buildTerminalWsUrl(apiClient.baseUrl, params)

        val reqBuilder = Request.Builder().url(url)
        apiClient.authToken?.let {
            reqBuilder.addHeader("Authorization", "Bearer $it")
        }

        var assignedId: String? = null

        val sinkRef = object : TerminalSink {
            var ws: WebSocket? = null

            override fun input(data: String) {
                val bytes = data.toByteArray(StandardCharsets.UTF_8)
                val frame = ByteArray(bytes.size + 1)
                frame[0] = INPUT_FRAME_TAG
                System.arraycopy(bytes, 0, frame, 1, bytes.size)
                ws?.send(frame.toByteString())
            }

            override fun resize(cols: Int, rows: Int) {
                val json = ConsoleJson.encodeToString(
                    TerminalResizeMessage.serializer(),
                    TerminalResizeMessage(cols = cols, rows = rows),
                )
                ws?.send(json)
            }

            override fun kill() {
                val json = ConsoleJson.encodeToString(
                    TerminalKillMessage.serializer(),
                    TerminalKillMessage(),
                )
                ws?.send(json)
            }

            override fun close() {
                ws?.close(1000, "normal")
            }
        }

        val listener = object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                sinkRef.ws = webSocket
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                try {
                    // Control JSON frames: spawned, exit, error, output
                    val root = ConsoleJson.parseToJsonElement(text)
                    val type = (root as? kotlinx.serialization.json.JsonObject)?.get("type")
                        ?.let { (it as? kotlinx.serialization.json.JsonPrimitive)?.content }

                    when (type) {
                        "spawned" -> {
                            val spawned = ConsoleJson.decodeFromString(TerminalSpawnedEvent.serializer(), text)
                            assignedId = spawned.id
                            sinks[spawned.id] = sinkRef
                            terminalState.set(
                                spawned.id,
                                TerminalRecord(
                                    id = spawned.id,
                                    projectId = projectId,
                                    status = TerminalStatus.Running,
                                    pid = spawned.pid,
                                    shell = spawned.shell,
                                    cwd = spawned.cwd,
                                    cols = spawned.cols,
                                    rows = spawned.rows,
                                ),
                            )
                            openingDeferreds.remove(cacheKey)
                            deferred.complete(spawned)
                        }
                        "exit" -> {
                            val exit = ConsoleJson.decodeFromString(TerminalExitEvent.serializer(), text)
                            val id = assignedId
                            if (id != null) {
                                terminalState.patch(id) { it.copy(status = TerminalStatus.Exited) }
                            }
                        }
                        "error" -> {
                            val err = ConsoleJson.decodeFromString(TerminalErrorEvent.serializer(), text)
                            val id = assignedId
                            if (id != null) {
                                terminalState.patch(id) { it.copy(status = TerminalStatus.Error, error = err.message) }
                            }
                            if (!deferred.isCompleted) {
                                openingDeferreds.remove(cacheKey)
                                deferred.completeExceptionally(Exception(err.message))
                            }
                        }
                        "output" -> {
                            val out = ConsoleJson.decodeFromString(TerminalOutputEvent.serializer(), text)
                            val id = assignedId
                            if (id != null) {
                                terminalState.appendOutput(id, out.data)
                            }
                        }
                    }
                } catch (_: Exception) {
                }
            }

            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                if (bytes.size <= 1) return
                val tag = bytes[0]
                if (tag == OUTPUT_FRAME_TAG) {
                    val rawData = bytes.substring(1).utf8()
                    val id = assignedId
                    if (id != null) {
                        terminalState.appendOutput(id, rawData)
                    }
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                val id = assignedId
                if (id != null) {
                    terminalState.patch(id) { it.copy(status = TerminalStatus.Error, error = t.message) }
                }
                if (!deferred.isCompleted) {
                    openingDeferreds.remove(cacheKey)
                    deferred.completeExceptionally(t)
                }
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                val id = assignedId
                if (id != null) {
                    sinks.remove(id)
                    terminalState.patch(id) { it.copy(status = TerminalStatus.Exited) }
                }
            }
        }

        httpClient.newWebSocket(reqBuilder.build(), listener)
        return deferred.await()
    }

    fun write(terminalId: String, data: String) {
        sinks[terminalId]?.input(data)
    }

    fun resize(terminalId: String, cols: Int, rows: Int) {
        terminalState.patch(terminalId) { it.copy(cols = cols, rows = rows) }
        sinks[terminalId]?.resize(cols, rows)
    }

    fun kill(terminalId: String) {
        val sink = sinks.remove(terminalId)
        sink?.kill()
        sink?.close()
        terminalState.remove(terminalId)
    }

    fun clearAll() {
        for ((_, sink) in sinks) {
            try { sink.close() } catch (_: Exception) {}
        }
        sinks.clear()
        openingDeferreds.clear()
        terminalState.clear()
    }
}
