package com.console.mobile.feature.computer

import android.annotation.SuppressLint
import android.util.Log
import android.webkit.ConsoleMessage
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.NewTheme
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

private const val COMPUTER_PLAYER_URL = "https://appassets.androidplatform.net/computer/player.html"
private const val COMPUTER_PLAYER_ASSET = "computer/player.html"

private class ComputerBridge(private val onStatus: (String) -> Unit) {
    @JavascriptInterface
    fun postMessage(json: String) {
        try {
            val root = Json.parseToJsonElement(json) as? JsonObject ?: return
            val type = root["type"]?.toString()?.trim('"')
            if (type == "status") {
                val status = root["status"]?.toString()?.trim('"') ?: ""
                onStatus(status)
            }
        } catch (_: Exception) {}
    }
}

@SuppressLint("SetJavaScriptEnabled")
@Composable
fun ComputerScreen(
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
) {
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val backendUrl = appState.backendUrl ?: ""
    var streamStatus by remember { mutableStateOf("idle") }
    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var pageReady by remember { mutableStateOf(false) }

    val streamUrl = remember(backendUrl) {
        if (backendUrl.isBlank()) "" else AppContainer.deviceRepository.getComputerStreamUrl()
    }

    LaunchedEffect(streamUrl, webViewRef, pageReady) {
        val wv = webViewRef ?: return@LaunchedEffect
        if (!pageReady || streamUrl.isBlank()) return@LaunchedEffect
        val config = buildJsonObject {
            put("streamUrl", streamUrl)
        }.toString()
        val escaped = config.replace("\\", "\\\\").replace("'", "\\'")
        wv.evaluateJavascript("window.__consoleComputer && window.__consoleComputer.start('$escaped')", null)
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .background(NewTheme.Background)
    ) {
        PageHeader(
            title = "Computer",
            onBack = onBack,
        )

        Box(
            modifier = Modifier
                .weight(1f)
                .fillMaxWidth(),
            contentAlignment = Alignment.Center,
        ) {
            AndroidView(
                factory = { ctx ->
                    WebView(ctx).apply {
                        settings.javaScriptEnabled = true
                        settings.domStorageEnabled = true
                        settings.mediaPlaybackRequiresUserGesture = false
                        settings.cacheMode = WebSettings.LOAD_NO_CACHE
                        settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
                        setBackgroundColor(0xFF0A0A0B.toInt())
                        addJavascriptInterface(ComputerBridge { s ->
                            streamStatus = s
                        }, "AndroidBridge")
                        webChromeClient = object : WebChromeClient() {
                            override fun onConsoleMessage(consoleMessage: ConsoleMessage): Boolean {
                                Log.d("ComputerPlayer", "${consoleMessage.message()} -- From line ${consoleMessage.lineNumber()}")
                                return true
                            }
                        }
                        webViewClient = object : WebViewClient() {
                            override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? {
                                if (request.url.toString() != COMPUTER_PLAYER_URL) return null
                                return WebResourceResponse("text/html", "utf-8", view.context.assets.open(COMPUTER_PLAYER_ASSET))
                            }

                            override fun onPageFinished(view: WebView, url: String?) {
                                if (url == COMPUTER_PLAYER_URL) pageReady = true
                            }
                        }
                        loadUrl(COMPUTER_PLAYER_URL)
                        webViewRef = this
                    }
                },
                onRelease = { wv ->
                    wv.destroy()
                    if (webViewRef === wv) webViewRef = null
                    pageReady = false
                    streamStatus = "idle"
                },
                modifier = Modifier.fillMaxSize(),
            )

            if (streamStatus != "streaming") {
                val msg = when (streamStatus) {
                    "idle", "connecting" -> "Connecting to remote desktop…"
                    "waiting for video" -> "Waiting for video…"
                    "disconnected" -> "Stream disconnected. Retrying…"
                    else -> streamStatus
                }
                Text(
                    text = msg,
                    color = if (streamStatus == "disconnected" || streamStatus == "error") NewTheme.Danger else NewTheme.TextSecondary,
                    fontSize = 13.sp,
                )
            }
        }

        // Quick key bar for mobile control
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .background(NewTheme.Card)
                .horizontalScroll(rememberScrollState())
                .padding(horizontal = 12.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            val keys = listOf(
                "Esc" to 0xff1b,
                "Tab" to 0xff09,
                "Ctrl" to 0xffe3,
                "Alt" to 0xffe9,
                "Super" to 0xffeb,
                "Enter" to 0xff0d,
                "Backspace" to 0xff08,
            )
            keys.forEach { (label, keysym) ->
                Text(
                    text = label,
                    color = NewTheme.TextPrimary,
                    fontSize = 12.sp,
                    modifier = Modifier
                        .clip(RoundedCornerShape(NewTheme.ChipRadius))
                        .background(NewTheme.Raised)
                        .clickable {
                            webViewRef?.evaluateJavascript(
                                "window.__consoleComputer && (window.__consoleComputer.sendKey($keysym, true), window.__consoleComputer.sendKey($keysym, false))",
                                null
                            )
                        }
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                )
            }
        }
    }

    DisposableEffect(Unit) {
        onDispose {
            webViewRef?.evaluateJavascript("window.__consoleComputer && window.__consoleComputer.stop()", null)
        }
    }
}