package com.console.mobile

/**
 * Spike binding for the Rust WebTransport client (plan Phase 3).
 *
 * The whole point of this route versus Cronet is that it has **no OS-version
 * floor** — the same `wtransport` stack the desktop dogfoods, compiled for
 * Android. This class is the entire JNI surface for the spike: one blocking
 * call that opens a session and reports whether the handshake completed.
 *
 * A production client would instead expose the session and its streams, with
 * Kotlin holding a `WtSession` handle and the framing living in Rust (the same
 * framing `apps/desktop/crates/console-core/src/net/wt_client.rs` uses). That
 * is the next step; this only answers "does it work on a device at all".
 *
 * Build the `.so` first: `apps/android/rust-wt-spike/build.sh`.
 */
object WtNative {
    init {
        System.loadLibrary("console_wt_android")
    }

    const val OK = 0
    const val ERR_BAD_ARG = -1
    const val ERR_RUNTIME = -2
    const val ERR_ENDPOINT = -3
    const val ERR_HANDSHAKE = -4
    const val ERR_CERT = -5

    /**
     * Opens one WebTransport session and reports whether the handshake
     * completed. Blocks until it does, so call it off the main thread.
     *
     * @param certSha256 hex SHA-256 of the server's DER certificate; empty
     *   uses platform roots. A self-signed dev certificate needs this.
     */
    external fun probeSession(
        origin: String,
        token: String,
        certSha256: String,
    ): Int

    /**
     * Build marker from the Rust side. Calling anything here first runs
     * `init`, so this also proves `System.loadLibrary` succeeded — a wrong ABI
     * or a missing `build.sh` staging step fails here rather than later.
     */
    external fun abiVersion(): Int

    fun describe(code: Int): String = when (code) {
        OK -> "session established"
        ERR_BAD_ARG -> "bad arguments"
        ERR_RUNTIME -> "could not start the async runtime"
        ERR_ENDPOINT -> "could not create the QUIC endpoint"
        ERR_HANDSHAKE -> "handshake failed"
        ERR_CERT -> "certificate hash must be 64 hex characters"
        else -> "unknown result $code"
    }

    /**
     * The most recent failure, or an empty string. The Rust side has no
     * logger inside the `.so`, so this is the only way a failure can explain
     * itself.
     */
    external fun lastError(): String
}