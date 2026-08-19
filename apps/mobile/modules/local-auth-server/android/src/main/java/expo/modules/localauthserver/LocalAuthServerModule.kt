package expo.modules.localauthserver

import android.util.Log
import androidx.core.os.bundleOf
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import fi.iki.elonen.NanoHTTPD
import java.util.concurrent.atomic.AtomicReference

/**
 * Expo native module that runs a tiny HTTP server on the device's localhost to
 * catch the OAuth loopback redirect (`http://localhost:PORT/callback`).
 *
 * Google's installed-app OAuth client IDs only permit `http://localhost:PORT`
 * redirect URIs. On Android, `localhost` resolves to the device itself, so a
 * NanoHTTPD server bound to `127.0.0.1` receives the redirect the browser
 * makes after the user authenticates. The server extracts `code` and `state`
 * from the query string, emits them to JS via an event, and serves a simple
 * success page to the browser.
 *
 * Lifecycle:
 *   startServer(port, callbackPath) → NanoHTTPD binds, listens for one GET.
 *   on redirect hit                  → emits "onAuthCallback" { code, state }
 *                                      and "onAuthComplete", then auto-stops.
 *   on bind failure                   → emits "onAuthError" { error }.
 *   stopServer()                      → tears down the listener if still alive.
 *
 * Only one server is alive at a time; starting a new one stops the previous.
 */
class LocalAuthServerModule : Module() {
  private val activeServer = AtomicReference<CallbackServer?>(null)

  override fun definition() = ModuleDefinition {
    Name("LocalAuthServerModule")

    Events(
      "onAuthCallback",
      "onAuthError",
      "onAuthComplete"
    )

    AsyncFunction("startServer") { port: Int, callbackPath: String ->
      // Stop any previous listener before starting a new one.
      activeServer.getAndSet(null)?.stopSafe()

      val server = CallbackServer(port, callbackPath) { code, state ->
        sendEvent(
          "onAuthCallback",
          bundleOf(
            "code" to code,
            "state" to (state ?: "")
          )
        )
        sendEvent("onAuthComplete", bundleOf("port" to port))
      }

      try {
        // start() opens the listening socket; on a taken port it throws
        // BindException, which we catch and surface to JS as an error event.
        server.start(NanoHTTPD.SOCKET_READ_TIMEOUT, false)
        activeServer.set(server)
        true
      } catch (e: Exception) {
        val msg = e.message ?: "Failed to bind localhost:$port"
        Log.w("LocalAuthServer", "bind failed: $msg")
        sendEvent("onAuthError", bundleOf("error" to msg))
        false
      }
    }

    AsyncFunction("stopServer") {
      val stopped = activeServer.getAndSet(null)?.stopSafe()
      stopped != null
    }

    AsyncFunction("isRunning") ->
      activeServer.get()?.isAlive() == true
  }

  /**
   * NanoHTTPD subclass that listens on 127.0.0.1 and matches a single path.
   * On the first matching GET it extracts `code`/`state`, invokes [onResult],
   * and returns a minimal success page. Non-matching paths get a 404.
   */
  private class CallbackServer(
    port: Int,
    private val callbackPath: String,
    private val onResult: (code: String, state: String?) -> Unit,
  ) : NanoHTTPD("127.0.0.1", port) {

    override fun serve(session: IHTTPSession): Response {
      val uri = session.uri ?: return newFixedLengthResponse(Response.Status.NOT_FOUND, MIME_HTML, "Not found")

      // Only the configured callback path is honoured.
      if (uri != callbackPath) {
        return newFixedLengthResponse(Response.Status.NOT_FOUND, MIME_HTML, "Not found")
      }

      // GET query params are parsed by NanoHTTPD into session.parameters.
      val params = session.parameters
      val code = params["code"]?.firstOrNull()
      val state = params["state"]?.firstOrNull()
      val error = params["error"]?.firstOrNull()

      // Always serve a success page so the browser tab doesn't show a raw
      // connection error — the user just sees "authenticated, return to app".
      val html = if (error != null) {
        ERROR_HTML.replace("{{error}}", error)
      } else if (code != null) {
        SUCCESS_HTML
      } else {
        "Missing code parameter."
      }

      // Emit before returning so JS can begin the token exchange immediately.
      if (code != null) {
        onResult(code, state)
      }
      // Provider errors (e.g. user denied consent) are surfaced via the
      // served HTML page; the JS side times out and the user retries. We
      // intentionally do not emit here so the callback contract stays
      // "code present → exchange started".

      return newFixedLengthResponse(Response.Status.OK, MIME_HTML, html)
    }

    fun stopSafe() {
      try {
        stop()
      } catch (e: Exception) {
        Log.w("LocalAuthServer", "stop failed: ${e.message}")
      }
    }
  }

  companion object {
    private const val SUCCESS_HTML = """<!DOCTYPE html>
<html>
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
    <title>Console Auth</title>
    <style>
      body{background:#0a0a0b;color:#fff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;padding:24px}
      .card{text-align:center;background:#121214;padding:32px 28px;border-radius:16px;border:1px solid #222;max-width:360px}
      h2{margin:0 0 10px;font-size:18px;font-weight:600}
      p{margin:0;font-size:13px;color:#888;line-height:1.5}
    </style>
  </head>
  <body>
    <div class="card">
      <h2>Authentication successful</h2>
      <p>You can close this tab and return to Console.</p>
    </div>
  </body>
</html>"""

    private const val ERROR_HTML = """<!DOCTYPE html>
<html>
  <head>
    <meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
    <title>Console Auth</title>
    <style>
      body{background:#0a0a0b;color:#fff;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;padding:24px}
      .card{text-align:center;background:#121214;padding:32px 28px;border-radius:16px;border:1px solid #3a1a1a;max-width:360px}
      h2{margin:0 0 10px;font-size:18px;font-weight:600;color:#f87171}
      p{margin:0;font-size:13px;color:#888;line-height:1.5}
    </style>
  </head>
  <body>
    <div class="card">
      <h2>Authentication failed</h2>
      <p>{{error}}</p>
    </div>
  </body>
</html>"""
  }
}
