package com.console.mobile.feature.settings

import android.content.Context
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import com.console.mobile.data.repo.AuthRepository
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketTimeoutException
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import kotlin.coroutines.coroutineContext
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext

/**
 * OAuth login for providers the server supports (codex, claude).
 *
 * The server registers *loopback* redirect URIs with each provider — codex is
 * `http://localhost:1455/auth/callback` and claude is
 * `http://localhost:53692/callback` (see the provider constants under
 * apps/server-go/internal/providers).
 * The provider therefore redirects the browser back to localhost **on this
 * device**, so we have to be listening on that port ourselves. A `console://`
 * custom scheme is never used by the provider and cannot be.
 *
 * This mirrors the desktop client, which binds a TcpListener on the port parsed
 * out of `redirectUri` (apps/desktop/src/state/auth.rs, login_oauth).
 *
 * Flow: /login/url -> bind loopback port -> open Custom Tab -> capture
 * code+state -> verify state -> POST /login/callback.
 *
 * Antigravity and Devin return 501 from the Go server; the error surfaces via
 * the caller's exception handler.
 */
object OAuthLoginLauncher {
    /** Matches the desktop client's 120s callback window. */
    private const val CALLBACK_TIMEOUT_MS = 120_000L

    /** Accept poll interval, kept short so cancellation and the deadline are responsive. */
    private const val ACCEPT_POLL_MS = 500L

    private data class Callback(val code: String, val state: String)

    private const val SUCCESS_PAGE =
        "<!DOCTYPE html><html><head><title>Signed in</title></head>" +
            "<body style=\"font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;margin:0;\">" +
            "<div style=\"text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;\">" +
            "<h2 style=\"margin:0 0 8px 0;color:#22c55e;\">Authentication successful!</h2>" +
            "<p style=\"margin:0;color:#a1a1aa;\">You can close this tab and return to Console.</p>" +
            "</div></body></html>"

    private const val FAILURE_PAGE =
        "<!DOCTYPE html><html><head><title>Sign-in failed</title></head>" +
            "<body style=\"font-family:-apple-system,BlinkMacSystemFont,sans-serif;background:#18181b;color:#f4f4f5;display:flex;align-items:center;justify-content:center;height:90vh;margin:0;\">" +
            "<div style=\"text-align:center;padding:32px;background:#27272a;border-radius:12px;border:1px solid #3f3f46;\">" +
            "<h2 style=\"margin:0 0 8px 0;color:#ef4444;\">Sign-in failed</h2>" +
            "<p style=\"margin:0;color:#a1a1aa;\">Return to Console and try again.</p>" +
            "</div></body></html>"

    /**
     * Runs the full interactive login. Suspends until the provider redirects
     * back and the server has exchanged the code. Throws on any failure.
     */
    suspend fun login(context: Context, authRepo: AuthRepository, provider: String) {
        val result = authRepo.getLoginUrl(provider)
        val port = redirectPort(result.redirectUri)

        withContext(Dispatchers.IO) {
            // Bind before opening the browser so a fast redirect can't be missed.
            val server = bindLoopback(port)
            try {
                withContext(Dispatchers.Main) {
                    CustomTabsIntent.Builder()
                        .setShowTitle(true)
                        .build()
                        .launchUrl(context, Uri.parse(result.authUrl))
                }
                val callback = awaitCallback(server, result.state)
                authRepo.submitCallback(provider, callback.code, callback.state)
            } finally {
                runCatching { server.close() }
            }
        }
    }

    /** Extracts the loopback port the provider will redirect to. */
    private fun redirectPort(redirectUri: String): Int =
        runCatching { URI(redirectUri).port }
            .getOrNull()
            ?.takeIf { it in 1..65535 }
            ?: throw IllegalStateException("Provider returned an unusable redirect URI: $redirectUri")

    private fun bindLoopback(port: Int): ServerSocket =
        runCatching { ServerSocket(port, 8, InetAddress.getByName("127.0.0.1")) }
            .recoverCatching { ServerSocket(port) } // fall back to all interfaces
            .getOrElse {
                throw IllegalStateException("Could not listen on OAuth callback port $port: ${it.message}", it)
            }

    private suspend fun awaitCallback(server: ServerSocket, expectedState: String): Callback {
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
        throw IllegalStateException("Timed out waiting for the provider to finish sign-in.")
    }

    /**
     * Handles one inbound request. Returns the callback on success, or null if
     * this was not the provider redirect (favicon probe, provider error page,
     * …) and we should keep waiting.
     */
    private fun handleRequest(client: Socket, expectedState: String): Callback? {
        client.soTimeout = 4_000
        // HTTP request lines are ISO-8859-1. Don't close the reader: the socket
        // stays open so we can still write the response.
        val reader = BufferedReader(InputStreamReader(client.getInputStream(), StandardCharsets.ISO_8859_1))
        val requestLine = try {
            reader.readLine()
        } catch (_: Exception) {
            null
        } ?: return null

        val query = requestLine.substringAfter('?', "").substringBefore(' ').trim()
        if (query.isEmpty()) {
            respond(client, 404, FAILURE_PAGE)
            return null
        }

        val params = query.split('&').mapNotNull { part ->
            if (part.isEmpty()) return@mapNotNull null
            val key = part.substringBefore('=')
            if (key.isEmpty()) null else decode(key) to decode(part.substringAfter('=', ""))
        }.toMap()

        val code = params["code"]
        val state = params["state"]
        if (code.isNullOrEmpty() || state.isNullOrEmpty()) {
            // Not the provider redirect. Answer so the tab doesn't hang.
            respond(client, 400, FAILURE_PAGE)
            return null
        }
        if (state != expectedState) {
            // Defence in depth — the server also rejects unknown states, but we
            // should never hand a mismatched code to it.
            respond(client, 400, FAILURE_PAGE)
            throw IllegalStateException("OAuth state mismatch — this sign-in was not started on this device.")
        }

        respond(client, 200, SUCCESS_PAGE)
        return Callback(code = code, state = state)
    }

    private fun respond(client: Socket, status: Int, body: String) {
        val reason = if (status == 200) "OK" else if (status == 404) "Not Found" else "Bad Request"
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
        runCatching { URLDecoder.decode(value, "UTF-8") }.getOrDefault(value)
}
