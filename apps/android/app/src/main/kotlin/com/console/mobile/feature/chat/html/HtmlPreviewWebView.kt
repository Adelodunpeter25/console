package com.console.mobile.feature.chat.html

import android.annotation.SuppressLint
import android.graphics.Color
import android.util.LruCache
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.webkit.JavascriptInterface
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import com.console.mobile.AppContainer
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicReference

/**
 * Process-lifetime cache for inline transcript WebViews and their measured
 * heights, so scrolling off-screen or navigating away and back keeps the
 * rendered page in memory without reloading.
 */
internal object InlineWebViewCache {
    private const val MAX_CACHED_WEBVIEWS = 12

    class Entry(
        val webView: WebView,
        val onHeight: AtomicReference<((Int) -> Unit)?> = AtomicReference(null),
        val onExternal: AtomicReference<((String) -> Unit)?> = AtomicReference(null),
    )

    private val heights = ConcurrentHashMap<String, Int>()

    private val cache = object : LruCache<String, Entry>(MAX_CACHED_WEBVIEWS) {
        override fun entryRemoved(evicted: Boolean, key: String, oldValue: Entry, newValue: Entry?) {
            if (evicted && newValue == null) {
                oldValue.webView.post {
                    (oldValue.webView.parent as? ViewGroup)?.removeView(oldValue.webView)
                    oldValue.webView.destroy()
                }
            }
        }
    }

    fun cachedHeight(path: String): Int? = heights[path]

    fun recordHeight(path: String, height: Int) {
        heights[path] = height
    }

    fun getOrCreate(
        key: String,
        create: () -> Entry,
    ): Entry {
        val existing = cache.get(key)
        if (existing != null) {
            (existing.webView.parent as? ViewGroup)?.removeView(existing.webView)
            return existing
        }
        return create().also { cache.put(key, it) }
    }
}

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
    nestedInList: Boolean = false,
    onContentHeight: ((Int) -> Unit)? = null,
) {
    val pageUrl = remember(path) { AppContainer.consoleApi.fsHtmlUrl(path, theme = "dark") }
    val basePage = remember(pageUrl) { pageUrl.substringBefore('#') }

    AndroidView(
        modifier = modifier,
        factory = { context ->
            if (nestedInList) {
                val entry = InlineWebViewCache.getOrCreate(pageUrl) {
                    val heightRef = AtomicReference(onContentHeight)
                    val externalRef = AtomicReference(onExternalUrl)
                    val wv = WebView(context.applicationContext).apply {
                        layoutParams = ViewGroup.LayoutParams(
                            ViewGroup.LayoutParams.MATCH_PARENT,
                            ViewGroup.LayoutParams.MATCH_PARENT,
                        )
                        setBackgroundColor(Color.TRANSPARENT)
                        // The card is sized to the page, so the page itself never scrolls
                        // and every drag belongs to the chat list.
                        isVerticalScrollBarEnabled = false
                        isHorizontalScrollBarEnabled = false
                        overScrollMode = View.OVER_SCROLL_NEVER
                        setOnTouchListener { _, event -> event.actionMasked == MotionEvent.ACTION_MOVE }
                        addJavascriptInterface(
                            object {
                                @JavascriptInterface
                                fun postMessage(message: String) {
                                    val height = runCatching {
                                        org.json.JSONObject(message).getJSONObject("params").getInt("height")
                                    }.getOrNull() ?: return
                                    InlineWebViewCache.recordHeight(path, height)
                                    post { heightRef.get()?.invoke(height) }
                                }
                            },
                            "ConsoleHost",
                        )
                        configurePreviewSettings(settings)
                        webViewClient = object : WebViewClient() {
                            override fun shouldOverrideUrlLoading(
                                view: WebView,
                                request: WebResourceRequest,
                            ): Boolean {
                                val target = request.url.toString()
                                if (target.substringBefore('#') == basePage) return false
                                externalRef.get()?.invoke(target)
                                return true
                            }
                        }
                        tag = pageUrl
                        loadUrl(pageUrl)
                    }
                    InlineWebViewCache.Entry(wv, heightRef, externalRef)
                }
                entry.onHeight.set(onContentHeight)
                entry.onExternal.set(onExternalUrl)
                entry.webView
            } else {
                WebView(context).apply {
                    layoutParams = ViewGroup.LayoutParams(
                        ViewGroup.LayoutParams.MATCH_PARENT,
                        ViewGroup.LayoutParams.MATCH_PARENT,
                    )
                    setBackgroundColor(Color.TRANSPARENT)
                    configurePreviewSettings(settings)
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
                    tag = pageUrl
                    loadUrl(pageUrl)
                }
            }
        },
        update = { view ->
            if (view.tag != pageUrl) {
                view.tag = pageUrl
                view.loadUrl(pageUrl)
            }
        },
        onRelease = { view ->
            if (nestedInList) {
                (view.parent as? ViewGroup)?.removeView(view)
            } else {
                view.destroy()
            }
        },
    )
}

@SuppressLint("SetJavaScriptEnabled")
private fun configurePreviewSettings(settings: WebSettings) {
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
}