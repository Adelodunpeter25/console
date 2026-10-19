package com.console.mobile.feature.browser

import android.webkit.WebView
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue

/** A main-frame load failure, renderable in place of the page. */
data class BrowserPageError(
    val url: String,
    val code: Int,
    val description: String,
)

/**
 * Live state of one browser surface: what the chrome paints and what the
 * WebView reports.
 *
 * The WebView itself stays private to this class — nothing outside
 * [BrowserWebView] should be able to navigate behind the state's back — and the
 * bridge methods are `internal` so only the view wiring drives it.
 */
@Stable
class BrowserState(initialUrl: String = "") {
    /** The address this surface opens with; loaded when the WebView attaches. */
    private val startUrl: String = normalizeBrowserUrl(initialUrl).orEmpty()

    /** The committed URL: what the address bar shows unless it is being edited. */
    var url by mutableStateOf(startUrl)
        internal set
    var title by mutableStateOf<String?>(null)
        internal set
    var progress by mutableStateOf(0)
        internal set
    var isLoading by mutableStateOf(false)
        internal set
    var canGoBack by mutableStateOf(false)
        internal set
    var canGoForward by mutableStateOf(false)
        internal set
    var error by mutableStateOf<BrowserPageError?>(null)
        internal set

    internal var webView: WebView? = null

    /** Load address-bar input after [normalizeBrowserUrl]; false = not a URL. */
    fun loadAddress(input: String): Boolean {
        val normalized = normalizeBrowserUrl(input) ?: return false
        load(normalized)
        return true
    }

    fun load(url: String) {
        error = null
        this.url = url
        // The previous page stays on screen while the new one loads.
        webView?.loadUrl(url)
    }

    fun reload() {
        error = null
        webView?.reload()
    }

    fun stop() {
        webView?.stopLoading()
    }

    fun goBack() {
        if (webView?.canGoBack() != true) return
        error = null
        webView?.goBack()
    }

    fun goForward() {
        if (webView?.canGoForward() != true) return
        error = null
        webView?.goForward()
    }

    internal fun attach(view: WebView) {
        webView = view
        // The initial URL is loaded here, not in the constructor: loadUrl
        // before a WebView exists goes nowhere.
        if (startUrl.isNotEmpty() && view.url == null) view.loadUrl(startUrl)
        url = view.url ?: startUrl
        syncNavigation(view)
    }

    internal fun detach(view: WebView) {
        if (webView === view) webView = null
        view.destroy()
    }

    internal fun onPageStarted(view: WebView, pageUrl: String) {
        if (pageUrl.isNotBlank()) url = pageUrl
        isLoading = true
        progress = 0
        error = null
        syncNavigation(view)
    }

    internal fun onPageFinished(view: WebView, pageUrl: String) {
        if (pageUrl.isNotBlank()) url = pageUrl
        isLoading = false
        progress = 100
        syncNavigation(view)
    }

    internal fun onPageError(view: WebView, pageUrl: String, code: Int, description: String) {
        isLoading = false
        error = BrowserPageError(pageUrl, code, description)
        syncNavigation(view)
    }

    internal fun syncNavigation(view: WebView) {
        canGoBack = view.canGoBack()
        canGoForward = view.canGoForward()
    }
}

@Composable
fun rememberBrowserState(initialUrl: String = ""): BrowserState =
    remember(initialUrl) { BrowserState(initialUrl) }
