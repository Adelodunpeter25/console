package com.console.mobile

import com.console.mobile.core.forward.LocalForwardManager
import java.io.IOException
import java.net.Socket
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.ByteString
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/**
 * The tunnel is the difference between a working preview and a dead URL, and
 * no screen exercises it yet. Each loopback connection must carry raw TCP
 * through one WebSocket, byte for byte, and must not outlive its socket.
 *
 * The stand-in server echoes every WebSocket frame back, which makes the
 * manager's loopback listener behave like an echo server on the other side of
 * a real tunnel handshake.
 */
class LocalForwardManagerTest {
    private lateinit var server: MockWebServer
    private lateinit var client: OkHttpClient

    @Before
    fun setUp() {
        server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse =
                MockResponse().withWebSocketUpgrade(
                    object : WebSocketListener() {
                        override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                            webSocket.send(bytes)
                        }
                    },
                )
        }
        server.start()
        client = OkHttpClient.Builder().readTimeout(0, TimeUnit.MILLISECONDS).build()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test(timeout = 30_000)
    fun pipesLoopbackBytesThroughTheTunnel() = runBlocking {
        val manager = LocalForwardManager(client, baseUrl = { server.url("/").toString() })
        try {
            val forward = manager.ensure(45999)
            assertNotNull(forward)
            forward!!
            assertEquals(45999, forward.remotePort)
            assertEquals("http://127.0.0.1:${forward.localPort}/", forward.localUrl)

            val payload = "GET / HTTP/1.1\r\nHost: dev\r\n\r\n".toByteArray()
            Socket("127.0.0.1", forward.localPort).use { socket ->
                socket.soTimeout = 5000
                socket.getOutputStream().write(payload)
                socket.getOutputStream().flush()

                val echoed = ByteArray(payload.size)
                var read = 0
                while (read < echoed.size) {
                    val count = socket.getInputStream().read(echoed, read, echoed.size - read)
                    if (count <= 0) break
                    read += count
                }
                assertEquals(String(payload, Charsets.UTF_8), String(echoed.copyOf(read), Charsets.UTF_8))
            }
        } finally {
            manager.clear()
        }
    }

    @Test(timeout = 30_000)
    fun reconcileStartsNewListenersAndDropsStaleOnes() = runBlocking {
        val manager = LocalForwardManager(client, baseUrl = { server.url("/").toString() })
        try {
            val both = manager.reconcile(listOf(45001, 45002))
            assertEquals(listOf(45001, 45002), both.map { it.remotePort })

            val remaining = manager.reconcile(listOf(45002))
            assertEquals(listOf(45002), remaining.map { it.remotePort })
            assertNull(manager.localUrl(45001))
            assertEquals(remaining.single().localUrl, manager.localUrl(45002))

            // The dropped listener is closed, so nothing can connect to it.
            val dropped = both.first { it.remotePort == 45001 }
            assertNull(connectOrNull(dropped.localPort))
        } finally {
            manager.clear()
        }
    }

    @Test(timeout = 30_000)
    fun reconcileIsIdempotentForPortsAlreadyForwarded() = runBlocking {
        val manager = LocalForwardManager(client, baseUrl = { server.url("/").toString() })
        try {
            val first = manager.reconcile(listOf(45003)).single()
            val second = manager.reconcile(listOf(45003)).single()
            assertEquals(first, second)
        } finally {
            manager.clear()
        }
    }

    @Test
    fun tunnelUrlFollowsTheBackendScheme() {
        val plain = LocalForwardManager(client, baseUrl = { "http://10.0.2.2:9090/" })
        assertEquals("ws://10.0.2.2:9090/api/ports/5173/tunnel", plain.tunnelUrl(5173))

        val secure = LocalForwardManager(client, baseUrl = { "https://dev.example.com" })
        assertEquals("wss://dev.example.com/api/ports/5173/tunnel", secure.tunnelUrl(5173))
    }

    private fun connectOrNull(port: Int): Socket? = try {
        Socket("127.0.0.1", port).also { it.close() }
    } catch (_: IOException) {
        null
    }
}
