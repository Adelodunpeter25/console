package com.console.mobile.feature.computer

import android.annotation.SuppressLint
import android.webkit.WebChromeClient
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.NewTheme

@SuppressLint("SetJavaScriptEnabled")
@Composable
fun ComputerScreen(
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
) {
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val backendUrl = appState.backendUrl ?: ""

    Column(
        modifier = modifier
            .fillMaxSize()
            .background(NewTheme.Background)
    ) {
        PageHeader(
            title = "Computer",
            onBack = onBack,
        )

        Box(modifier = Modifier.fillMaxSize()) {
            val playerUrl = remember(backendUrl) {
                if (backendUrl.isBlank()) ""
                else backendUrl.trimEnd('/') + "/api/computer/player"
            }

            if (playerUrl.isNotBlank()) {
                AndroidView(
                    factory = { context ->
                        WebView(context).apply {
                            settings.apply {
                                javaScriptEnabled = true
                                domStorageEnabled = true
                                mediaPlaybackRequiresUserGesture = false
                                cacheMode = WebSettings.LOAD_NO_CACHE
                            }
                            webViewClient = WebViewClient()
                            webChromeClient = WebChromeClient()
                            loadUrl(playerUrl)
                        }
                    },
                    update = { view ->
                        if (view.url != playerUrl) {
                            view.loadUrl(playerUrl)
                        }
                    },
                    modifier = Modifier.fillMaxSize()
                )
            }
        }
    }
}
