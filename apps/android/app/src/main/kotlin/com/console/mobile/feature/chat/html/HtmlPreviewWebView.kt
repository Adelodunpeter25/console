package com.console.mobile.feature.chat.html

import android.annotation.SuppressLint
import android.graphics.Color
import android.view.ViewGroup
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import com.console.mobile.AppContainer

/**
 * Sandboxed WebView for rendering workspace HTML pages served by
 * `/api/fs/file/html`. File/content access and DOM storage are disabled;
 * external links are handed off to [onExternalUrl] instead of navigating
 * inside the preview.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
fun HtmlPreviewWebView(
    path: String,
    modifier: Modifier = Modifier,
    onExternalUrl: ((String) -> Unit)? = null,
) {
    val pageUrl = remember(path) { AppContainer.consoleApi.fsHtmlUrl(path, theme = "dark") }
    val basePage = remember(pageUrl) { pageUrl.substringBefore('#') }

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
                    domStorageEnabled = false
                    allowFileAccess = false
                    allowContentAccess = false
                    mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
                    cacheMode = WebSettings.LOAD_DEFAULT
                    setSupportMultipleWindows(false)
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
                        if (target.substringBefore('#') == basePage) return false
                        onExternalUrl?.invoke(target)
                        return true
                    }
                }
                loadUrl(pageUrl)
            }
        },
        update = { view ->
            if (view.tag != pageUrl) {
                view.tag = pageUrl
                view.loadUrl(pageUrl)
            }
        },
    )
}
