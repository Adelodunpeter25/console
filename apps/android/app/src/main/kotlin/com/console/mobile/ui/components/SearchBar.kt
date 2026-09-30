package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors

/**
 * The search input itself: pill, magnifier, text, and a clear affordance.
 *
 * Built on `BasicTextField` with an explicit `textStyle` rather than
 * Material3's `OutlinedTextField`. The M3 field resolves its text colour from
 * `OutlinedTextFieldDefaults.colors(...)`, whose focused/unfocused text colour
 * params are deprecated in Material3 1.4 and no longer reach the painter — the
 * text renders in the scheme's `onSurface` and vanishes on a dark background.
 * Setting the colour on the text style is unconditional, so it cannot drift.
 */
@Composable
fun ConsoleSearchField(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "Search",
) {
    Row(
        modifier = modifier.height(48.dp)
            // 12dp rather than a full pill: at 48dp tall, CircleShape reads as a
            // lozenge that does not match the 12-16dp radii used elsewhere.
            .clip(RoundedCornerShape(12.dp))
            .background(ConsoleColors.Card)
            .padding(horizontal = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Filled.Search, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(18.dp))
        BasicTextField(
            value = value,
            onValueChange = onValueChange,
            modifier = Modifier.weight(1f).padding(start = 8.dp),
            singleLine = true,
            textStyle = androidx.compose.ui.text.TextStyle(color = ConsoleColors.TextPrimary, fontSize = 14.sp),
            cursorBrush = SolidColor(ConsoleColors.TextPrimary),
            decorationBox = { innerTextField ->
                Box(contentAlignment = Alignment.CenterStart) {
                    if (value.isEmpty()) {
                        Text(placeholder, color = ConsoleColors.TextMuted, fontSize = 14.sp, maxLines = 1)
                    }
                    innerTextField()
                }
            },
        )
        if (value.isNotBlank()) {
            Box(
                modifier = Modifier.size(24.dp).clip(CircleShape)
                    .clickable(onClickLabel = "Clear") { onValueChange("") },
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Filled.Close, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(14.dp))
            }
        }
    }
}

/**
 * Port of components/common/search-bar.tsx.
 * Sticky bottom search field + round compose button.
 */
@Composable
fun ConsoleSearchBar(
    value: String,
    onValueChange: (String) -> Unit,
    modifier: Modifier = Modifier,
    placeholder: String = "Search threads",
    onComposePress: (() -> Unit)? = null,
    composeEnabled: Boolean = true,
) {
    Row(
        modifier = modifier.fillMaxWidth()
            // Sticky bottom bar holding the only text input on this screen, so
            // it has to ride above the IME or the keyboard hides it while the
            // user is typing a query. Navigation bars are excluded because the
            // window already insets for them. Matches Composer.kt.
            .windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars))
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        ConsoleSearchField(
            value = value,
            onValueChange = onValueChange,
            placeholder = placeholder,
            modifier = Modifier.weight(1f).padding(end = 8.dp),
        )
        if (onComposePress != null) {
            Surface(
                shape = CircleShape,
                color = Color.Black,
                border = BorderStroke(1.5.dp, ConsoleColors.Border),
                modifier = Modifier.size(48.dp),
            ) {
                IconButton(onClick = onComposePress, enabled = composeEnabled) {
                    Icon(Icons.Filled.Edit, contentDescription = "New chat", tint = Color.White)
                }
            }
        }
    }
}
