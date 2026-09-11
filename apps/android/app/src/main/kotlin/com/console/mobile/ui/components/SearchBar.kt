package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
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
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        TextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text(placeholder, color = ConsoleColors.TextMuted) },
            leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null, tint = ConsoleColors.TextMuted) },
            trailingIcon = if (value.isNotBlank()) {
                {
                    IconButton(onClick = { onValueChange("") }) {
                        Icon(Icons.Filled.Close, contentDescription = "Clear", tint = ConsoleColors.TextSecondary)
                    }
                }
            } else null,
            singleLine = true,
            shape = CircleShape,
            colors = TextFieldDefaults.colors(
                focusedContainerColor = ConsoleColors.Card,
                unfocusedContainerColor = ConsoleColors.Card,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
                focusedTextColor = ConsoleColors.TextPrimary,
                unfocusedTextColor = ConsoleColors.TextPrimary,
                cursorColor = ConsoleColors.TextPrimary,
            ),
            modifier = Modifier.weight(1f).padding(end = 12.dp),
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
