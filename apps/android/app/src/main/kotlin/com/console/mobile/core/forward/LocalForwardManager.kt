package com.console.mobile.core.forward

import java.io.IOException
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.util.Collections
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString
import okio.ByteString.Companion.toByteString

/** A remote dev port exposed as a loopback URL on this device. */
data class LocalForward(
    val remotePort: Int,
    val localPort: Int,
    val localUrl: String,
)

/**
 * Local port forwarding: the mobile twin of the desktop `LocalForwardManager`.
 *
 * The server can only report that remote `8000` exists — its localhost is the
 * dev machine, not this phone — and the proxy URL it hands out is a LAN
 * address, which is useless once the phone is on a VPN. So this manager owns
 * one loopback listener per remote port and pipes every accepted TCP
 * connection through the server's tunnel WebSocket:
 *
 *   WebView http://127.0.0.1:<local> <-> ServerSocket <-> ws(s) tunnel <-> server 127.0.0.1:<remote>
 *
 * Raw TCP is forwarded byte for byte, so absolute asset paths, service
 * workers, and HMR websockets behave exactly as if the dev server ran on the
 * phone. The WebView runs in this process, so a loopback listener is enough.
 *
 * All socket work is blocking IO on [scope]; WebSocket callbacks arrive on
 * OkHttp's dispatcher threads. Nothing here touches the UI.
 *
 * A backend running on the device itself (Termux, say) needs no special case:
 * the tunnel simply loops through it.
 */
class LocalForwardManager(
    private val client: OkHttpClient,
    /** Backend base URL, read on demand so a settings change applies at once. */
    private val baseUrl: () -> String,
    /** Optional bearer token for the tunnel handshake. */
    private val authToken: () -> String? = { null },
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    private val mutex = Mutex()
    private val active = LinkedHashMap<Int, ActiveForward>()

    private val _forwards = MutableStateFlow<List<LocalForward>>(emptyList())

    /** Every live forward, ordered by remote port. */
    val forwards: StateFlow<List<LocalForward>> = _forwards.asStateFlow()

    /** The loopback URL for [remotePort], or null when it is not forwarded. */
    fun localUrl(remotePort: Int): String? =
        _forwards.value.firstOrNull { it.remotePort == remotePort }?.localUrl

    /**
     * Start (or reuse) the listener for [remotePort]. Null when no loopback
     * port could be bound — the caller should surface that as a failure rather
     * than navigating to a dead URL.
     */
    suspend fun ensure(remotePort: Int): LocalForward? {
        if (remotePort !in 1..65535) return null
        return mutex.withLock {
            active[remotePort]?.let { return@withLock it.forward }

            val listener = bindLoopback(remotePort) ?: return@withLock null
            val wsUrl = tunnelUrl(remotePort)
            val sockets = Collections.synchronizedSet(mutableSetOf<Socket>())
            val connections = CoroutineScope(SupervisorJob() + Dispatchers.IO)
            val acceptJob = scope.launch {
                while (isActive) {
                    val socket = try {
                        listener.accept()
                    } catch (_: IOException) {
                        break // listener closed by remove()/clear()
                    }
                    connections.launch { pipe(socket, wsUrl, sockets) }
                }
            }
            val forward = LocalForward(
                remotePort = remotePort,
                localPort = listener.localPort,
                localUrl = "http://127.0.0.1:${listener.localPort}/",
            )
            active[remotePort] = ActiveForward(forward, listener, acceptJob, connections, sockets)
            publish()
            forward
        }
    }

    /**
     * Reconcile listeners with the latest remote port list: start the new ones,
     * drop the ones that disappeared. Returns the live forwards.
     */
    suspend fun reconcile(remotePorts: List<Int>): List<LocalForward> {
        val wanted = remotePorts.filter { it in 1..65535 }.toSet()
        val stale = mutex.withLock { active.keys.filter { it !in wanted } }
        for (port in stale) remove(port)
        for (port in wanted) ensure(port)
        return _forwards.value
    }

    /** Stop forwarding [remotePort] and drop its in-flight connections. */
    suspend fun remove(remotePort: Int) {
        val entry = mutex.withLock {
            active.remove(remotePort)?.also { publish() }
        } ?: return
        entry.shutdown()
    }

    /** Stop every forward (settings change, sign-out, process teardown). */
    suspend fun clear() {
        val entries = mutex.withLock {
            val all = active.values.toList()
            active.clear()
            publish()
            all
        }
        for (entry in entries) entry.shutdown()
    }

    /**
     * Tear a forward down. The listener and the live sockets close before the
     * jobs are cancelled: a coroutine parked in a blocking `accept()` or
     * `read()` is not interrupted by cancellation, it needs the socket to fail
     * — and `cancelAndJoin` would otherwise wait on it forever.
     */
    private suspend fun ActiveForward.shutdown() {
        runCatching { listener.close() }
        acceptJob.cancelAndJoin()
        for (socket in sockets.toList()) runCatching { socket.close() }
        sockets.clear()
        connections.cancel()
    }

    private fun publish() {
        _forwards.value = active.values.map { it.forward }.sortedBy { it.remotePort }
    }

    /** Tunnel WebSocket URL for a remote port, derived from the backend URL. */
    fun tunnelUrl(remotePort: Int): String {
        val base = baseUrl().trim().trimEnd('/')
        val wsBase = when {
            base.startsWith("https://", ignoreCase = true) -> "wss://" + base.substring(8)
            base.startsWith("http://", ignoreCase = true) -> "ws://" + base.substring(7)
            else -> "ws://$base"
        }
        return "$wsBase/api/ports/$remotePort/tunnel"
    }

    /**
     * Bind loopback, preferring the remote port number so URLs stay memorable,
     * and falling back to an OS-picked port when it is taken.
     */
    private fun bindLoopback(remotePort: Int): ServerSocket? {
        val loopback = InetAddress.getByName("127.0.0.1")
        val preferred = ServerSocket()
        return try {
            preferred.reuseAddress = true
            preferred.bind(InetSocketAddress(loopback, remotePort))
            preferred
        } catch (_: IOException) {
            runCatching { preferred.close() }
            try {
                ServerSocket(0, BACKLOG, loopback)
            } catch (_: IOException) {
                null
            }
        }
    }

    /**
     * Pipe one local TCP connection through one tunnel WebSocket: each WS
     * message is one TCP chunk in either direction, byte for byte.
     */
    private fun pipe(socket: Socket, wsUrl: String, sockets: MutableSet<Socket>) {
        val closed = CountDownLatch(1)
        var ws: WebSocket? = null
        sockets.add(socket)
        try {
            val request = Request.Builder().url(wsUrl).apply {
                authToken()?.takeIf { it.isNotBlank() }?.let { addHeader("Authorization", "Bearer $it") }
            }.build()
            ws = client.newWebSocket(
                request,
                object : WebSocketListener() {
                    override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                        writeToSocket(webSocket, socket, bytes.toByteArray())
                    }

                    override fun onMessage(webSocket: WebSocket, text: String) {
                        writeToSocket(webSocket, socket, text.toByteArray(Charsets.UTF_8))
                    }

                    override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                        webSocket.close(code, reason)
                    }

                    override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                        // The tunnel ended: unblock the socket read loop too.
                        runCatching { socket.close() }
                        closed.countDown()
                    }

                    override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                        runCatching { socket.close() }
                        closed.countDown()
                    }
                },
            )

            val input = socket.getInputStream()
            val buffer = ByteArray(CHUNK)
            while (true) {
                val read = try {
                    input.read(buffer)
                } catch (_: IOException) {
                    -1
                }
                if (read <= 0) break
                // send() is false once the queue is closed or full: stop reading
                // rather than buffering a response nobody can receive.
                if (!ws.send(buffer.toByteString(0, read))) break
            }
            ws.close(1000, null)
        } catch (_: Exception) {
            runCatching { ws?.cancel() }
        } finally {
            // Tear both ends down: a closed socket makes the listener's writes
            // fail, and cancel() stops a read blocked on the tunnel, so neither
            // a blocked read nor a pending callback outlives this connection.
            sockets.remove(socket)
            runCatching { socket.close() }
            ws?.cancel()
            runCatching { closed.await(CLOSE_GRACE_MS, TimeUnit.MILLISECONDS) }
        }
    }

    private fun writeToSocket(webSocket: WebSocket, socket: Socket, bytes: ByteArray) {
        try {
            val output = socket.getOutputStream()
            output.write(bytes)
            output.flush()
        } catch (_: IOException) {
            // The local connection is gone: drop the tunnel with it.
            runCatching { socket.close() }
            webSocket.cancel()
        }
    }

    private class ActiveForward(
        val forward: LocalForward,
        val listener: ServerSocket,
        val acceptJob: Job,
        val connections: CoroutineScope,
        /** Sockets with a pipe in flight: closed on teardown so blocked reads unblock. */
        val sockets: MutableSet<Socket>,
    )

    private companion object {
        const val BACKLOG = 50
        /** TCP read size: one WS binary frame per chunk. */
        const val CHUNK = 16 * 1024
        const val CLOSE_GRACE_MS = 500L
    }
}
