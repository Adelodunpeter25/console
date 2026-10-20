package com.console.mobile

import java.io.Closeable

/**
 * WebTransport streams for mobile, backed by the **same** `console-core`
 * implementation the desktop client uses (plan Phase 3), cross-compiled for
 * Android.
 *
 * There is no second protocol implementation here: framing, the session cache,
 * certificate pinning and reconnect all live in Rust. This class is the Kotlin
 * face of it — open a stream, read frames, send client frames.
 *
 * Frames arrive in the same shape the SSE path already produces
 * (`seq`, event name, raw body), so the existing parsers in
 * `ChatStreamClient` and the repository layer are unchanged.
 */
object WtNative {
    /**
     * True when the staged Rust library loaded.
     *
     * `libconsole_wt_android.so` is produced by `rust-wt/build.sh` and is
     * gitignored on purpose, so any build that skipped that step — CI, a fresh
     * clone — carries no library at all, as do the 32-bit ABIs the script never
     * stages. That is not fatal: WebTransport is optional and every caller
     * falls back to SSE/WS, exactly as it does for a server with no WT
     * listener. It must not be fatal either — an unguarded `loadLibrary` in an
     * object initializer takes the whole app down from
     * `AppContainer.initialize`, before any UI exists to explain it. App startup
     * logs the downgrade; nothing else here touches Android, so the flag is also
     * what a JVM test sees.
     */
    val available: Boolean = try {
        System.loadLibrary("console_wt_android")
        true
    } catch (_: UnsatisfiedLinkError) {
        false
    }

    const val OK = 0

    /**
     * Build marker from the Rust side. Every `external` here needs
     * [available] first; without it the JNI symbol does not resolve.
     */
    external fun abiVersion(): Int

    /** The most recent failure, or an empty string. */
    external fun lastError(): String

    /** Stream kinds, matching `console.v1.WtStreamKind`. */
    external fun kindChatRun(): Int

    external fun kindTerminal(): Int

    external fun kindWatch(): Int

    /**
     * Opens a stream, connecting the shared session if it is not up yet.
     * Returns a handle, or 0 on failure (see [lastError]).
     *
     * @param streamId chat runs use the session id; other kinds pass an empty
     *   string.
     * @param requestJson protojson request body for the stream.
     * @param authToken the paired device token. The WebTransport handshake is
     *   authenticated by it; an empty string means "not paired" and the open
     *   fails so the caller falls back to the authenticated legacy path.
     */
    external fun openStream(
        apiUrl: String,
        origin: String,
        certSha256: String,
        kind: Int,
        streamId: String,
        requestType: String,
        requestJson: String,
        authToken: String,
    ): Long

    /**
     * Blocking read of one frame. Returns `null` on timeout — the reader loop
     * should just continue — or an empty array when the stream ended or broke
     * (see [lastError]).
     *
     * Frame layout: `[u32BE typeLen][type][u64BE seq][payload]`.
     */
    external fun readFrame(handle: Long, timeoutMs: Int): ByteArray?

    /** Write one client frame: terminal input, resize, kill. Returns [OK]. */
    external fun sendFrame(handle: Long, type: String, payload: ByteArray): Int

    /**
     * Drop the stream, optionally sending a final client frame first (`kill`
     * for terminals, empty for the rest).
     */
    external fun closeStream(handle: Long, type: String): Int

    /** Tear every stream down — call when the app's transport goes away. */
    external fun closeAll()

    /** Human-readable for a JNI/stream result, for logs and test failures. */
    fun describe(handle: Long): String = when {
        !available -> "WebTransport unavailable on this build"
        handle == OK.toLong() -> "session established"
        handle == 0L -> "stream could not be opened: ${lastError()}"
        else -> "stream handle $handle"
    }
}

/** One frame off a WebTransport stream: the SSE frame shape. */
data class WtFrame(val seq: Long, val type: String, val payload: ByteArray) {
    /** The body as text. Frame bodies are protojson, except terminal output. */
    fun text(): String = String(payload, Charsets.UTF_8)

    override fun equals(other: Any?): Boolean =
        other is WtFrame && seq == other.seq && type == other.type && payload.contentEquals(other.payload)

    override fun hashCode(): Int =
        (seq.hashCode() * 31 + type.hashCode()) * 31 + payload.contentHashCode()
}

/**
 * A live WebTransport stream.
 *
 * Reading is deliberately polling rather than a JNI callback: a callback would
 * need `AttachCurrentThread` bookkeeping on every tokio worker thread, and the
 * app already drives streams from background coroutines. [readNext] blocks
 * briefly, so a loop of it is cheap and never spins.
 *
 * Always [close] it — the server-side resource (a run, a PTY) lives until the
 * stream goes away.
 */
class WtStream private constructor(
    val handle: Long,
    private val closeFrame: String,
) : Closeable {
    private val closed = java.util.concurrent.atomic.AtomicBoolean(false)

    /**
     * Next frame, or `null` if none arrived within [timeoutMs]. Throws
     * [IllegalStateException] if the stream ended or broke, with the reason
     * from [WtNative.lastError].
     */
    fun readNext(timeoutMs: Int = 100): WtFrame? {
        val raw = WtNative.readFrame(handle, timeoutMs) ?: return null
        if (raw.isEmpty()) {
            throw IllegalStateException(
                "WebTransport stream ended: ${WtNative.lastError()}",
            )
        }
        return decode(raw)
    }

    /** Send a client frame. [input] is raw PTY bytes for terminals. */
    fun send(type: String, payload: ByteArray = ByteArray(0)): Int =
        WtNative.sendFrame(handle, type, payload)

    fun sendInput(bytes: ByteArray) = send(FRAME_INPUT, bytes)

    fun sendInput(text: String) = sendInput(text.toByteArray(Charsets.UTF_8))

    fun sendResize(cols: Int, rows: Int) {
        // protojson TerminalResize: {"cols":…,"rows":…} — int32 fields, and
        // pbjson omits them only when zero, which the server reads as unset.
        send(FRAME_RESIZE, """{"cols":$cols,"rows":$rows}""".toByteArray(Charsets.UTF_8))
    }

    fun kill() {
        send(FRAME_KILL)
        close()
    }

    override fun close() {
        if (closed.compareAndSet(false, true)) {
            WtNative.closeStream(handle, closeFrame)
        }
    }

    companion object {
        const val FRAME_INPUT = "input"
        const val FRAME_RESIZE = "resize"
        const val FRAME_KILL = "kill"

        /** Request frame types, matching the server's stream switch. */
        const val REQUEST_SPAWN = "spawn"
        const val REQUEST_START = "start"
        const val REQUEST_ATTACH = "attach"
        const val REQUEST_WATCH = "watch"

        /**
         * Opens a stream, or returns null with the reason in
         * [WtNative.lastError] — which is the normal outcome when the server
         * is not serving WebTransport and the caller should use legacy.
         */
        fun open(
            apiUrl: String,
            origin: String,
            certSha256: String,
            kind: Int,
            streamId: String = "",
            requestType: String,
            requestJson: String,
            authToken: String = "",
            closeFrame: String = "",
        ): WtStream? {
            if (!WtNative.available) return null
            val handle = WtNative.openStream(
                apiUrl,
                origin,
                certSha256,
                kind,
                streamId,
                requestType,
                requestJson,
                authToken,
            )
            if (handle == 0L) return null
            return WtStream(handle, closeFrame)
        }

        /** Split `[u32BE typeLen][type][u64BE seq][payload]`. */
        private fun decode(raw: ByteArray): WtFrame {
            val buffer = java.nio.ByteBuffer.wrap(raw)
            val typeLength = buffer.int
            val type = ByteArray(typeLength).also { buffer.get(it) }
            val seq = buffer.long
            val payload = ByteArray(buffer.remaining()).also { buffer.get(it) }
            return WtFrame(seq, String(type, Charsets.UTF_8), payload)
        }
    }
}