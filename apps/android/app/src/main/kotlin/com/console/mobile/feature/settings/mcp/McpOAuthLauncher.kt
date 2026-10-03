package com.console.mobile.feature.settings.mcp

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import com.console.mobile.data.model.McpServerEntry
import com.console.mobile.data.repo.McpRepository
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketTimeoutException
import java.nio.charset.StandardCharsets
import kotlin.coroutines.coroutineContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext

/**
 * Browser OAuth for MCP servers, mirroring [OAuthLoginLauncher] but against
 * the MCP endpoints: the app binds its own loopback, passes it as
 * `redirectUri` on connect, opens the server-issued `authUrl`, captures the
 * provider redirect locally, and forwards code+state to
 * `POST /api/mcp/servers/:id/oauth/callback`. The server performs the code
 * exchange and persists the token — the secret never touches the client.
 *
 * The loopback port is ephemeral (unlike provider login's fixed ports), so
 * concurrent provider and MCP sign-ins can never steal each other's
 * redirects.
 */
object McpOAuthLauncher {
    /** Matches the server's 5-minute auth window so the client never gives up first. */
    private const val CALLBACK_TIMEOUT_MS = 300_000L
    private const val ACCEPT_POLL_MS = 500L

    private data class Callback(val code: String?, val state: String, val error: String?)

    /**
     * Runs the full connect-and-authorize flow: binds a loopback, starts the
     * server-side connect with it as `redirectUri`, opens the issued
     * `authUrl` when the row reaches `needs_auth`, captures the provider
     * redirect locally, forwards it, and waits for the terminal state.
     * Returns true when the row ends connected.
     */
    suspend fun connectAndAuthorize(
        context: Context,
        mcpRepo: McpRepository,
        serverId: String,
    ): Boolean {
        val (server, redirectUri) = bindCallback() ?: run {
            // No loopback: plain server-side connect, previous behavior.
            val settled = mcpRepo.startConnect(serverId, null)
            return settled?.isConnected == true
        }
        try {
            val settled = mcpRepo.startConnect(serverId, redirectUri) ?: return false
            if (settled.isConnected) return true
            val authUrl = settled.authUrl
            if (!settled.needsAuth || authUrl.isNullOrBlank()) return false
            val expectedState = stateOf(authUrl) ?: return false
            withContext(Dispatchers.Main) {
                val tab = CustomTabsIntent.Builder()
                    .setShowTitle(true)
                    .build()
                if (context !is Activity) {
                    tab.intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                }
                tab.launchUrl(context, Uri.parse(authUrl))
            }
            val callback = awaitCallback(server, expectedState) ?: return false
            val forwarded = mcpRepo.forwardOAuthCallback(
                serverId,
                state = callback.state,
                code = callback.code,
                error = callback.error,
            )
            if (!forwarded) return false
            return mcpRepo.awaitTerminal(serverId)?.isConnected == true
        } finally {
            runCatching { server.close() }
        }
    }

    private fun stateOf(authUrl: String): String? {
        val query = authUrl.substringAfter('?', "").trim()
        if (query.isEmpty()) return null
        return query.split('&')
            .firstOrNull { it.startsWith("state=") }
            ?.substringAfter('=')
            ?.let { decode(it) }
            ?.ifEmpty { null }
    }

    /**
     * Binds an ephemeral loopback port and returns its callback URL for
     * `redirectUri`. The socket stays open in the returned pair until the
     * caller closes it after the flow settles.
     */
    fun bindCallback(): Pair<ServerSocket, String>? {
        val server = runCatching { ServerSocket(0, 8, InetAddress.getByName("127.0.0.1")) }
            .recoverCatching { ServerSocket(0) }
            .getOrNull() ?: return null
        val port = server.localPort.takeIf { it in 1..65535 } ?: run {
            runCatching { server.close() }
            return null
        }
        return server to "http://127.0.0.1:$port/callback"
    }

    private suspend fun awaitCallback(server: ServerSocket, expectedState: String): Callback? {
        val deadline = System.currentTimeMillis() + CALLBACK_TIMEOUT_MS
        while (System.currentTimeMillis() < deadline) {
            coroutineContext.ensureActive()
            server.soTimeout = minOf(deadline - System.currentTimeMillis(), ACCEPT_POLL_MS).toInt().coerceAtLeast(1)
            val client = try {
                server.accept()
            } catch (_: SocketTimeoutException) {
                continue
            }
            client.use { socket -> handleRequest(socket, expectedState)?.let { return it } }
        }
        return null
    }

    private fun handleRequest(client: Socket, expectedState: String): Callback? {
        client.soTimeout = 4_000
        val reader = BufferedReader(InputStreamReader(client.getInputStream(), StandardCharsets.ISO_8859_1))
        val requestLine = try {
            reader.readLine()
        } catch (_: Exception) {
            null
        } ?: return null

        val query = requestLine.substringAfter('?', "").substringBefore(' ').trim()
        if (query.isEmpty()) {
            respond(client, 404, "Not found")
            return null
        }
        val params = query.split('&').mapNotNull { part ->
            if (part.isEmpty()) return@mapNotNull null
            val key = part.substringBefore('=')
            if (key.isEmpty()) null else key to part.substringAfter('=', "")
        }.toMap()

        val state = params["state"]?.let { decode(it) }.orEmpty()
        if (state.isEmpty() || state != expectedState) {
            respond(client, 400, "Unknown sign-in request. Return to Console and try again.")
            return null
        }
        val code = params["code"]?.let { decode(it) }.orEmpty().ifEmpty { null }
        val error = params["error"]?.let { decode(it) }.orEmpty().ifEmpty { null }
        if (code == null && error == null) {
            respond(client, 400, "Sign-in failed. Return to Console and try again.")
            return null
        }
        respond(client, 200, "Authentication successful! You can close this tab and return to Console.")
        return Callback(code = code, state = state, error = error)
    }

    private fun respond(client: Socket, status: Int, message: String) {
        val reason = if (status == 200) "OK" else if (status == 404) "Not Found" else "Bad Request"
        val body = "<html><body style=\"font-family:sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;margin:0;\"><div>$message</div></body></html>"
        val bodyBytes = body.toByteArray(StandardCharsets.UTF_8)
        val head = buildString {
            append("HTTP/1.1 ").append(status).append(' ').append(reason).append("\r\n")
            append("Content-Type: text/html; charset=utf-8\r\n")
            append("Content-Length: ").append(bodyBytes.size).append("\r\n")
            append("Connection: close\r\n\r\n")
        }.toByteArray(StandardCharsets.ISO_8859_1)
        runCatching {
            val out = client.getOutputStream()
            out.write(head)
            out.write(bodyBytes)
            out.flush()
        }
    }

    private fun decode(value: String): String =
        runCatching { java.net.URLDecoder.decode(value, "UTF-8") }.getOrDefault(value)
}
