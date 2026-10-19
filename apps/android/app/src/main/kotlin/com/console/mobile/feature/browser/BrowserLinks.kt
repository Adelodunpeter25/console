package com.console.mobile.feature.browser

import android.util.Log
import androidx.compose.ui.platform.UriHandler

private const val LOG_TAG = "ConsoleBrowser"

/**
 * True for the links the in-app browser loads itself.
 *
 * Chat is a stream of URLs to forwarded dev ports and tunnels on the same
 * server this client is already talking to, so those belong in the app.
 * Everything else — `mailto:`, `tel:`, `intent:` — is a job for a system app,
 * which is where every link went before there was a browser.
 */
fun opensInBrowser(url: String): Boolean =
    url.startsWith("http://", ignoreCase = true) || url.startsWith("https://", ignoreCase = true)

/**
 * The `LocalUriHandler` for a subtree: http(s) links open in the in-app
 * browser, everything else keeps going to the system.
 *
 * Compose's own text links (`LinkAnnotation.Url`, which is what the markdown
 * renderer emits for `[text](url)`, autolinks and bare URLs) dispatch through
 * `LocalUriHandler`, so providing this one over a screen reroutes every link it
 * draws without the renderer knowing anything changed.
 */
class BrowserUriHandler(
    private val system: UriHandler,
    private val openInBrowser: (String) -> Unit,
) : UriHandler {
    override fun openUri(uri: String) {
        if (opensInBrowser(uri)) openInBrowser(uri) else handOffToSystem(system, uri)
    }
}

/**
 * Hands a URL to the system, tolerating "no app for this".
 *
 * Link text is arbitrary — an agent writes `intent://` or a made-up scheme
 * about as often as it writes a real URL — and a bare
 * `ActivityNotFoundException` from inside a bubble would take the app down.
 */
fun handOffToSystem(handler: UriHandler, url: String) {
    runCatching { handler.openUri(url) }
        .onFailure { Log.w(LOG_TAG, "no app handles $url") }
}
