package com.console.mobile.feature.browser

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Globe

/**
 * A browser surface: chrome, a [BrowserWebView], and in-place pages for
 * "nothing open yet" and "the load failed".
 *
 * Placement is the caller's call — this component needs a [BrowserState] (from
 * [rememberBrowserState]), a size, and nothing else. [toolbarTrailing] is where
 * a close button or an "open externally" action goes, and [onExternalUrl]
 * receives schemes the WebView cannot render (`mailto:`, `tel:`, ...).
 *
 * [onRetry] exists because a failed load is often a failed tunnel: the caller
 * may need to re-establish the forward before reloading.
 */
@Composable
fun BrowserScreen(
    state: BrowserState,
    modifier: Modifier = Modifier,
    toolbarTrailing: (@Composable RowScope.() -> Unit)? = null,
    onExternalUrl: ((String) -> Unit)? = null,
    onRetry: (() -> Unit)? = null,
) {
    // System back walks the history before it leaves the surface.
    BackHandler(enabled = state.canGoBack) { state.goBack() }

    Column(modifier.fillMaxSize().background(NewTheme.Background)) {
        BrowserToolbar(state, trailing = toolbarTrailing)
        Box(Modifier.weight(1f).fillMaxWidth()) {
            BrowserWebView(state, Modifier.fillMaxSize(), onExternalUrl)
            val error = state.error
            if (error != null) {
                BrowserErrorPage(error, onRetry = onRetry ?: state::reload)
            } else if (state.url.isEmpty()) {
                BrowserStartPage()
            }
        }
    }
}

@Composable
private fun BrowserStartPage() {
    Box(
        modifier = Modifier.fillMaxSize().background(NewTheme.Background),
        contentAlignment = Alignment.Center,
    ) {
        EmptyView(
            title = "Nothing open",
            description = "Type a dev server address above — a forwarded port's URL works best.",
            icon = TablerIcons.Outline.Globe,
        )
    }
}

@Composable
private fun BrowserErrorPage(error: BrowserPageError, onRetry: () -> Unit) {
    Box(
        modifier = Modifier.fillMaxSize().background(NewTheme.Background),
        contentAlignment = Alignment.Center,
    ) {
        EmptyView(
            title = "Can't open this page",
            description = error.description.ifBlank { error.url },
            icon = TablerIcons.Outline.AlertTriangle,
            iconTint = NewTheme.Danger,
            action = {
                ActionButton(
                    text = "Try again",
                    onClick = onRetry,
                    kind = ActionButtonKind.Primary,
                )
            },
        )
    }
}
