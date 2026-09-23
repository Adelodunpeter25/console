package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
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
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier.weight(1f).padding(end = 8.dp).height(48.dp)
                .clip(CircleShape).background(ConsoleColors.Card)
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
