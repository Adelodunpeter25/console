package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import io.github.lyxnx.compose.ui.tablericons.outline.X

/**
 * The search input itself: a rounded field with a magnifier, the text, and a
 * clear button once there is something to clear.
 *
 * Built on `BasicTextField` with an explicit `textStyle` rather than Material3's
 * `OutlinedTextField`: the M3 field's text colour comes from colour params that
 * no longer reach the painter in Material3 1.4, so the text falls back to the
 * scheme's `onSurface` and vanishes on a dark background. A colour set on the
 * text style cannot drift.
 */
@Composable
fun SearchInput(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "Search",
) {
    Row(
        modifier = modifier.height(48.dp)
            .clip(RoundedCornerShape(NewTheme.FieldRadius))
            .background(NewTheme.Card)
            .padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.Search, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(18.dp))
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier.weight(1f).padding(start = 10.dp),
            singleLine = true,
            textStyle = TextStyle(color = NewTheme.TextPrimary, fontSize = 15.sp),
            cursorBrush = SolidColor(NewTheme.Accent),
            decorationBox = { innerTextField ->
                Box(contentAlignment = Alignment.CenterStart) {
                    if (value.isEmpty()) Text(placeholder, color = NewTheme.TextMuted, fontSize = 15.sp, maxLines = 1)
                    innerTextField()
                }
            },
        )
        if (value.isNotBlank()) {
            Box(
                modifier = Modifier.size(24.dp).clip(CircleShape).clickable(onClickLabel = "Clear") { onValueChange("") },
                contentAlignment = Alignment.Center,
            ) {
                Icon(TablerIcons.Outline.X, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(14.dp))
            }
        }
    }
}

/**
 * Sticky bottom search: a [SearchInput] plus an optional round compose button.
 * It holds the only text input on its screen, so it rides above the keyboard
 * (the navigation bars are excluded because the window already insets for them).
 */
@Composable
fun SearchBar(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "Search threads",
    onComposePress: (() -> Unit)? = null,
    composeEnabled: Boolean = true,
) {
    Row(
        modifier = modifier.fillMaxWidth()
            .windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars))
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SearchInput(value = value, onValueChange = onValueChange, placeholder = placeholder, modifier = Modifier.weight(1f).padding(end = 8.dp))
        if (onComposePress != null) {
            Box(
                modifier = Modifier.size(48.dp).clip(CircleShape).background(NewTheme.Card)
                    .clickable(enabled = composeEnabled, onClickLabel = "New chat", onClick = onComposePress),
                contentAlignment = Alignment.Center,
            ) {
                Icon(TablerIcons.Outline.Edit, contentDescription = "New chat", tint = NewTheme.TextPrimary)
            }
        }
    }
}
