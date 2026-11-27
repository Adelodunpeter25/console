package com.console.mobile.feature.chat.html

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Code
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical

/**
 * Full-screen sandboxed preview for a rendered HTML page, with quick actions
 * to switch to the code editor or open in the system browser.
 */
@Composable
fun HtmlPreviewScreen(
    path: String,
    title: String,
    onBack: () -> Unit,
    onOpenEditor: (String) -> Unit,
    onOpenExternalUrl: (String) -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }
    val headerTitle = title.ifBlank { path.substringAfterLast('/') }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(
            title = headerTitle,
            subtitle = path.substringAfterLast('/'),
            onBack = onBack,
            actions = {
                Box {
                    IconButton(onClick = { menuOpen = true }) {
                        Icon(
                            TablerIcons.Outline.DotsVertical,
                            contentDescription = "More options",
                            tint = NewTheme.TextPrimary,
                            modifier = Modifier.size(20.dp),
                        )
                    }
                    OverflowMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        OverflowMenuItem(
                            label = "Open in editor",
                            icon = TablerIcons.Outline.Code,
                            onClick = {
                                menuOpen = false
                                onOpenEditor(path)
                            },
                        )
                    }
                }
            },
        )
        HtmlPreviewWebView(
            path = path,
            modifier = Modifier.weight(1f).fillMaxWidth(),
            onExternalUrl = onOpenExternalUrl,
        )
    }
}