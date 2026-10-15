package com.console.mobile.data.stream

import com.console.mobile.WtFrame
import com.console.mobile.WtStream
import com.console.mobile.data.model.AgentSessionEvent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import com.console.mobile.data.api.ConsoleJson

/**
 * WebTransport transport selection for mobile (plan Phase 3).
 *
 * One rule, mirroring the desktop client: **WebTransport first, SSE/WS when
 * the server cannot serve it.** Nothing above this layer knows which answered
 * — [ChatStreamClient] returns the same [StreamOutcome] either way.
 *
 * Unlike Cronet, this path has no OS-version floor: the transport is the same
 * `console-core` Rust client the desktop dogfoods, cross-compiled into the
 * app. So there is no "too old to try" case, only "this server has no
 * WebTransport listener", which shows up as a failed handshake and a fall back.
 *
 * ### When WebTransport is attempted
 *
 * Only when the API origin is TLS, or [originOverride] names one. WebTransport
 * mandates TLS, so probing a plaintext server can never succeed and would cost
 * the first stream a dial timeout on every app start — that is every plain
 * `make dev-server` boot.
 */
object WtTransport {
    /**
     * WebTransport origin when it is not the API origin, e.g. a dev server
     * serving plain HTTP on 3003 and QUIC on 14443.
     *
     * Null in normal use: production serves HTTP/3 on the API's own origin.
     * The settings UI should own this; until then it is set here so a dev
     * build can reach a WT server.
     */
    @Volatile
    var originOverride: String? = null

    /**
     * Hex SHA-256 of the server's DER certificate, for a self-signed dev
     * certificate. Empty uses the platform trust store, which is what any
     * publicly-trusted deployment wants.
     */
    @Volatile
    var certSha256: String = ""

    /** The WT origin for [baseUrl], or null when WT should not be attempted. */
    fun originFor(baseUrl: String): String? {
        originOverride?.let { return it }
        if (!baseUrl.startsWith("https://")) return null
        return baseUrl.trimEnd('/')
    }

    /**
     * A chat-run stream over WebTransport, or null when this server cannot
     * serve it (caller falls back to SSE).
     */
    fun openChatRun(baseUrl: String, sessionId: String, requestType: String, requestJson: String): WtStream? {
        val origin = originFor(baseUrl) ?: return null
        return WtStream.open(
            apiUrl = baseUrl.trimEnd('/'),
            origin = origin,
            certSha256 = certSha256,
            kind = WtNativeKind.chatRun(),
            streamId = sessionId,
            requestType = requestType,
            requestJson = requestJson,
        )
    }

    /**
     * `WtNativeKind` is a thin alias so this file does not have to know the
     * JNI object's shape; it exists to keep the stream-kind numbers in Rust.
     */
    object WtNativeKind {
        fun chatRun(): Int = com.console.mobile.WtNative.kindChatRun()
    }

    /**
     * Wrap a WT stream in the same [Flow] the SSE path returns, so callers
     * above the transport cannot tell them apart.
     *
     * Reads block briefly rather than spinning, and a frame that does not
     * decode is ignored — both matching the SSE reader's behavior.
     */
    fun stream(
        baseUrl: String,
        sessionId: String,
        requestType: String,
        requestJson: String,
    ): Flow<StreamOutcome>? {
        val stream = openChatRun(baseUrl, sessionId, requestType, requestJson) ?: return null
        return callbackFlow {
            val reader = launch(Dispatchers.IO) {
                var outcome: StreamOutcome? = null
                try {
                    while (true) {
                        val frame = stream.readNext(READ_TIMEOUT_MS) ?: continue
                        if (frame.type == "error") {
                            // The server answers a rejected request this way once
                            // the stream is accepted, so there is no HTTP status
                            // left to read. The reason is in the frame itself;
                            // `WtNative.lastError()` is about the *transport* and
                            // would still be holding an earlier failure.
                            outcome = StreamOutcome.End(StreamEnd.Failed(errorMessage(frame)))
                            break
                        }
                        val event = try {
                            ConsoleJson.decodeFromString<AgentSessionEvent>(frame.text())
                        } catch (_: Exception) {
                            continue // malformed frame: parity with the SSE reader
                        }
                        trySend(StreamOutcome.Frame(SseStreamFrame(frame.seq, event)))
                    }
                } catch (e: Exception) {
                    outcome = StreamOutcome.End(StreamEnd.Failed(e.message ?: "WebTransport stream error"))
                }
                if (outcome != null) trySend(outcome)
                close()
            }
            awaitClose {
                reader.cancel()
                stream.close()
            }
        }
    }

    /** Close every WT stream — call when the app's server changes. */
    fun reset() {
        com.console.mobile.WtNative.closeAll()
    }

    /**
     * A watch stream over WebTransport, or null when this server cannot serve
     * it (caller falls back to SSE).
     *
     * Mirrors the SSE watches: one target per stream, each frame named, and a
     * frame that does not decode is ignored rather than fatal. [decode]
     * belongs to the caller because the payload types are repository-owned
     * (Moshi adapters) and this file has no business knowing them.
     */
    fun <T> watch(
        baseUrl: String,
        requestJson: String,
        frameType: String,
        decode: (String) -> T?,
    ): Flow<T>? {
        val origin = originFor(baseUrl) ?: return null
        val stream = WtStream.open(
            apiUrl = baseUrl.trimEnd('/'),
            origin = origin,
            certSha256 = certSha256,
            kind = com.console.mobile.WtNative.kindWatch(),
            streamId = "",
            requestType = WtStream.REQUEST_WATCH,
            requestJson = requestJson,
        ) ?: return null
        return callbackFlow {
            val reader = launch(Dispatchers.IO) {
                try {
                    while (true) {
                        val frame = stream.readNext(READ_TIMEOUT_MS) ?: continue
                        if (frame.type == "error") break // server rejected the watch
                        if (frame.type != frameType) continue
                        // Malformed payloads are skipped, matching the SSE readers.
                        decode(frame.text())?.let { trySend(it) }
                    }
                } catch (_: Exception) {
                    // Stream ended or broke; a watch has nothing to report.
                }
                close()
            }
            awaitClose {
                reader.cancel()
                stream.close()
            }
        }
    }

    /**
     * Watch request bodies. Targets are the proto enum **names**, matching what
     * the server's protojson unmarshaller accepts, so renumbering the schema
     * cannot silently watch the wrong thing.
     */
    fun watchRequest(
        target: String,
        path: String? = null,
        projectId: String? = null,
        runId: String? = null,
    ): String = buildJsonObject {
        put("target", JsonPrimitive(target))
        path?.let { put("path", JsonPrimitive(it)) }
        projectId?.let { put("projectId", JsonPrimitive(it)) }
        runId?.let { put("runId", JsonPrimitive(it)) }
    }.toString()

    const val TARGET_NOTIFICATIONS = "WT_WATCH_TARGET_NOTIFICATIONS"
    const val TARGET_GIT_STATUS = "WT_WATCH_TARGET_GIT_STATUS"
    const val TARGET_FS = "WT_WATCH_TARGET_FS"

    /** Frame names the server sends for each target. */
    const val FRAME_NOTIFICATION = "notification"
    const val FRAME_GIT_STATUS = "gitStatus"
    const val FRAME_FS_CHANGE = "fsChange"

    private const val READ_TIMEOUT_MS = 250
}

/**
 * Pull the message out of an `error` frame. The body is a protojson
 * `ErrorPayload` (`{"message": …}`); raw text is the fallback, so a shape
 * change still surfaces something instead of an empty error.
 */
private fun errorMessage(frame: WtFrame): String {
    val text = frame.text()
    return try {
        ConsoleJson.parseToJsonElement(text)
            .jsonObject["message"]?.jsonPrimitive?.content ?: text
    } catch (_: Exception) {
        text
    }
}

/**
 * Translate the REST run body into the WebTransport start payload.
 *
 * Both sides are protojson over the same `console.v1` messages, so this is a
 * field rename rather than a second DTO mapping: the REST body calls the
 * prompt `prompt`, the WT `WtChatRunStart` calls it `text`. Optional fields are
 * passed through untouched — both encoders omit unset values.
 */
internal fun runBodyToWtStart(restBodyJson: String, sessionId: String): String {
    val body = ConsoleJson.parseToJsonElement(restBodyJson).jsonObject
    val start = buildJsonObject {
        (body["prompt"] as? JsonPrimitive)?.let { put("text", it) }
        for (key in listOf(
            "contextFiles", "modelId", "provider", "approvalMode", "thinkingLevel", "attachments",
        )) {
            body[key]?.let { put(key, it) }
        }
    }
    return buildJsonObject {
        put("sessionId", JsonPrimitive(sessionId))
        put("start", start)
    }.toString()
}

/** The WebTransport attach payload — mirrors `?since=`. */
internal fun attachRequestJson(sessionId: String, since: Long?): String = buildJsonObject {
    put("sessionId", JsonPrimitive(sessionId))
    put("attach", buildJsonObject {
        since?.let { put("since", JsonPrimitive(it)) }
    })
}.toString()