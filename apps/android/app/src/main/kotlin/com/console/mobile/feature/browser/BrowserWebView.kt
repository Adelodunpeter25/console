package com.console.mobile.feature.browser

import android.annotation.SuppressLint
import android.graphics.Bitmap
import android.graphics.Color
import android.util.Log
import android.view.ViewGroup
import android.webkit.ConsoleMessage
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView

private const val BROWSER_LOG_TAG = "ConsoleBrowser"

/**
 * The WebView behind a [BrowserState].
 *
 * Kept close to the Devices player's settings: javascript and DOM storage on,
 * no file access, one window (popups stay in the main frame). Non-web schemes
 * (`mailto:`, `tel:`, `intent:`, `market:`) are handed to [onExternalUrl]
 * instead of rendering a blank page.
 *
 * Deliberately not covered yet: downloads, file chooser, and SSL-error
 * prompts.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
fun BrowserWebView(
    state: BrowserState,
    modifier: Modifier = Modifier,
    /** Non-web schemes; null ignores them. */
    onExternalUrl: ((String) -> Unit)? = null,
) {
    AndroidView(
        modifier = modifier,
        factory = { context ->
            WebView(context).apply {
                layoutParams = ViewGroup.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT,
                )
                setBackgroundColor(Color.TRANSPARENT)
                settings.apply {
                    javaScriptEnabled = true
                    domStorageEnabled = true
                    // Dev servers mix http pages with ws:// sockets, and the
                    // strict default blocks the socket: the page stalls with no
                    // visible error.
                    mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
                    cacheMode = WebSettings.LOAD_DEFAULT
                    allowFileAccess = false
                    allowContentAccess = false
                    setSupportMultipleWindows(false)
                    // Pinch-zoom matters for previews that ignore the viewport.
                    setSupportZoom(true)
                    builtInZoomControls = true
                    displayZoomControls = false
                    loadsImagesAutomatically = true
                }
                webViewClient = object : WebViewClient() {
                    override fun shouldOverrideUrlLoading(
                        view: WebView,
                        request: WebResourceRequest,
                    ): Boolean {
                        val target = request.url.toString()
                        if (isWebUrl(target)) return false
                        onExternalUrl?.invoke(target)
                        return true
                    }

                    override fun onPageStarted(view: WebView, url: String?, favicon: Bitmap?) {
                        state.onPageStarted(view, url.orEmpty())
                    }

                    override fun onPageFinished(view: WebView, url: String?) {
                        state.onPageFinished(view, url.orEmpty())
                    }

                    override fun doUpdateVisitedHistory(view: WebView, url: String?, isReload: Boolean) {
                        state.syncNavigation(view)
                    }

                    override fun onReceivedError(
                        view: WebView,
                        request: WebResourceRequest,
                        error: WebResourceError,
                    ) {
                        if (!request.isForMainFrame) return
                        state.onPageError(
                            view = view,
                            pageUrl = request.url.toString(),
                            code = error.errorCode,
                            description = error.description?.toString().orEmpty(),
                        )
                    }

                    override fun onReceivedHttpError(
                        view: WebView,
                        request: WebResourceRequest,
                        errorResponse: WebResourceResponse,
                    ) {
                        // A 404 from a dev server is still a rendered page: only
                        // transport-level failures become error pages above.
                        if (request.isForMainFrame) state.syncNavigation(view)
                    }
                }
                webChromeClient = object : WebChromeClient() {
                    override fun onProgressChanged(view: WebView, newProgress: Int) {
                        state.progress = newProgress
                    }

                    override fun onReceivedTitle(view: WebView, title: String?) {
                        state.title = title
                    }

                    override fun onConsoleMessage(consoleMessage: ConsoleMessage): Boolean {
                        Log.d(
                            BROWSER_LOG_TAG,
                            "${consoleMessage.message()} @ ${consoleMessage.sourceId()}:${consoleMessage.lineNumber()}",
                        )
                        return true
                    }
                }
                state.attach(this)
            }
        },
        onRelease = { view -> state.detach(view) },
    )
}
