package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.model.INPUT_FRAME_TAG
import com.console.mobile.data.model.OUTPUT_FRAME_TAG
import com.console.mobile.data.model.TerminalSpawnParams
import com.console.mobile.data.model.buildTerminalWsUrl
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.TerminalKill
import console.v1.TerminalResize
import console.v1.TerminalServerMessage
import console.v1.TerminalSpawned
import com.console.mobile.data.store.TerminalRecord
import com.console.mobile.data.store.TerminalStateHolder
import com.console.mobile.data.store.TerminalStatus
import java.nio.charset.StandardCharsets
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeout
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

/** Upper bound on how long a terminal may take to spawn before we give up. */
private const val TERMINAL_OPEN_TIMEOUT_MS = 20_000L

class TerminalRepository(
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
    private val terminalState: TerminalStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    private val sinks = ConcurrentHashMap<String, TerminalSink>()
    private val openingDeferreds = ConcurrentHashMap<String, CompletableDeferred<TerminalSpawned>>()

    suspend fun openTerminal(
        projectId: String,
        cwd: String,
        cols: Int = 80,
        rows: Int = 24,
        label: String? = null,
        shell: String? = null,
    ): TerminalSpawned {
        val cacheKey = "$projectId::$cwd"
        val existing = openingDeferreds[cacheKey]
        // Bounded too: the first caller owns cleanup, but a joiner must not be
        // able to hang forever either.
        if (existing != null) return withTimeout(TERMINAL_OPEN_TIMEOUT_MS) { existing.await() }

        val deferred = CompletableDeferred<TerminalSpawned>()
        val wireMoshi: Moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
        val serverAdapter = wireMoshi.adapter(TerminalServerMessage::class.java)
        val resizeAdapter = wireMoshi.adapter(TerminalResize::class.java)
        val killAdapter = wireMoshi.adapter(TerminalKill::class.java)
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
                ws?.send(resizeAdapter.toJson(TerminalResize(cols = cols, rows = rows)))
            }

            override fun kill() {
                ws?.send(killAdapter.toJson(TerminalKill()))
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
                    // Control frames arrive as the oneof shape ({"spawned":{...}}).
                    val msg = serverAdapter.fromJson(text) ?: return
                    when (val event = msg.event) {
                        is TerminalServerMessage.Event.Spawned -> {
                            val spawned = event.value
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
                        is TerminalServerMessage.Event.Exit -> {
                            val id = assignedId
                            if (id != null) {
                                terminalState.patch(id) { it.copy(status = TerminalStatus.Exited) }
                            }
                        }
                        is TerminalServerMessage.Event.Error -> {
                            val message = event.value.message
                            val id = assignedId
                            if (id != null) {
                                terminalState.patch(id) { it.copy(status = TerminalStatus.Error, error = message) }
                            }
                            if (!deferred.isCompleted) {
                                openingDeferreds.remove(cacheKey)
                                deferred.completeExceptionally(Exception(message))
                            }
                        }
                        is TerminalServerMessage.Event.Output -> {
                            val id = assignedId
                            if (id != null) {
                                terminalState.appendOutput(id, event.value.data_)
                            }
                        }
                        null -> {
                            // Unknown control frame; binary output keeps flowing.
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
                // The socket can close before "spawned" ever arrives — spawn limit
                // reached, backend restarted, upgrade rejected after accept. Without
                // completing here, openTerminal() waits forever and, worse, the dead
                // deferred stays cached under this project::cwd so every later retry
                // awaits the same never-completing future.
                if (!deferred.isCompleted) {
                    openingDeferreds.remove(cacheKey)
                    deferred.completeExceptionally(
                        IllegalStateException("Terminal closed before it finished starting (code $code)."),
                    )
                }
            }
        }

        // Never let a failed or silent open leave a dead deferred cached: clean it
        // up on any throw, including our own timeout and a bad URL in newWebSocket.
        return try {
            httpClient.newWebSocket(reqBuilder.build(), listener)
            withTimeout(TERMINAL_OPEN_TIMEOUT_MS) { deferred.await() }
        } catch (e: Throwable) {
            if (!deferred.isCompleted) {
                openingDeferreds.remove(cacheKey)
                deferred.completeExceptionally(e)
            }
            throw e
        }
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
