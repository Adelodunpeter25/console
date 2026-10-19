package com.console.mobile.feature.browser

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowLeft
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowRight
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import io.github.lyxnx.compose.ui.tablericons.outline.Lock
import io.github.lyxnx.compose.ui.tablericons.outline.Reload
import io.github.lyxnx.compose.ui.tablericons.outline.X

/**
 * Browser chrome: back/forward, the address field, reload-or-stop, and a
 * hairline progress bar while a page loads.
 *
 * [trailing] is the caller's slot for whatever else the surrounding screen
 * wants (a close button, "open externally", ...) — this component owns no
 * navigation of its own.
 */
@Composable
fun BrowserToolbar(
    state: BrowserState,
    modifier: Modifier = Modifier,
    trailing: (@Composable RowScope.() -> Unit)? = null,
) {
    // While the field is focused it holds the user's text; on blur it falls
    // back to the committed URL, like a desktop address bar.
    var draft by remember { mutableStateOf<String?>(null) }
    val focusManager = LocalFocusManager.current

    Column(modifier.fillMaxWidth().background(NewTheme.Background)) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 2.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = state::goBack, enabled = state.canGoBack) {
                Icon(
                    TablerIcons.Outline.ArrowLeft,
                    contentDescription = "Back",
                    tint = if (state.canGoBack) NewTheme.TextPrimary else NewTheme.TextGhost,
                    modifier = Modifier.size(20.dp),
                )
            }
            IconButton(onClick = state::goForward, enabled = state.canGoForward) {
                Icon(
                    TablerIcons.Outline.ArrowRight,
                    contentDescription = "Forward",
                    tint = if (state.canGoForward) NewTheme.TextPrimary else NewTheme.TextGhost,
                    modifier = Modifier.size(20.dp),
                )
            }
            AddressField(
                value = draft ?: state.url,
                secure = isSecureUrl(state.url),
                onValueChange = { draft = it },
                onSubmit = {
                    val input = draft ?: state.url
                    draft = null
                    state.loadAddress(input)
                    focusManager.clearFocus()
                },
                // Blur reverts to the committed URL instead of stranding a
                // half-typed address on screen.
                onBlur = { draft = null },
                modifier = Modifier.weight(1f).padding(horizontal = 4.dp),
            )
            if (state.isLoading) {
                IconButton(onClick = state::stop) {
                    Icon(
                        TablerIcons.Outline.X,
                        contentDescription = "Stop",
                        tint = NewTheme.TextPrimary,
                        modifier = Modifier.size(20.dp),
                    )
                }
            } else {
                IconButton(onClick = state::reload) {
                    Icon(
                        TablerIcons.Outline.Reload,
                        contentDescription = "Reload",
                        tint = NewTheme.TextPrimary,
                        modifier = Modifier.size(20.dp),
                    )
                }
            }
            trailing?.invoke(this)
        }
        LoadBar(progress = state.progress, visible = state.isLoading)
    }
}

@Composable
private fun LoadBar(progress: Int, visible: Boolean) {
    Box(Modifier.fillMaxWidth().height(2.dp)) {
        if (visible) {
            Box(
                Modifier
                    .fillMaxWidth((progress / 100f).coerceIn(0f, 1f))
                    .height(2.dp)
                    .background(NewTheme.Accent),
            )
        }
    }
}

@Composable
private fun AddressField(
    value: String,
    secure: Boolean,
    onValueChange: (String) -> Unit,
    onSubmit: () -> Unit,
    onBlur: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .height(40.dp)
            .clip(RoundedCornerShape(NewTheme.InputRadius))
            .background(NewTheme.Field)
            .padding(horizontal = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (secure) TablerIcons.Outline.Lock else TablerIcons.Outline.Globe,
            contentDescription = null,
            tint = NewTheme.TextMuted,
            modifier = Modifier.size(14.dp),
        )
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier
                .weight(1f)
                .padding(start = 8.dp)
                .onFocusChanged { if (!it.isFocused) onBlur() },
            singleLine = true,
            textStyle = TextStyle(color = NewTheme.TextPrimary, fontSize = 14.sp),
            cursorBrush = SolidColor(NewTheme.Accent),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go),
            keyboardActions = KeyboardActions(onGo = { onSubmit() }),
            decorationBox = { innerTextField ->
                Box(contentAlignment = Alignment.CenterStart) {
                    if (value.isEmpty()) {
                        Text("Enter address", color = NewTheme.TextMuted, fontSize = 14.sp, maxLines = 1)
                    }
                    innerTextField()
                }
            },
        )
    }
}
