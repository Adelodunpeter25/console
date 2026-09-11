package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors

/**
 * Port of components/layout/screen-header.tsx.
 * Title (22sp bold) + optional subtitle, back button, settings + headerActions.
 * Safe-area is handled by the Scaffold / WindowInsets — no manual paddingTop.
 */
@Composable
fun ScreenHeader(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    onBack: (() -> Unit)? = null,
    showSettings: Boolean = false,
    onSettingsPress: (() -> Unit)? = null,
    actions: (@Composable RowScope.() -> Unit)? = null,
) {
    Row(
        modifier = modifier.padding(horizontal = 16.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            Surface(
                shape = CircleShape,
                color = ConsoleColors.Card,
                border = BorderStroke(1.dp, ConsoleColors.Border),
                modifier = Modifier.padding(end = 12.dp).size(40.dp),
            ) {
                IconButton(onClick = onBack) {
                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = ConsoleColors.TextPrimary)
                }
            }
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = title,
                color = ConsoleColors.TextPrimary,
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    color = ConsoleColors.TextSecondary,
                    fontSize = 12.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        if (actions != null) {
            Row(verticalAlignment = Alignment.CenterVertically, content = actions)
        }
        if (showSettings) {
            Surface(
                shape = CircleShape,
                color = ConsoleColors.Card,
                border = BorderStroke(1.dp, ConsoleColors.Border),
                modifier = Modifier.size(40.dp),
            ) {
                IconButton(onClick = { onSettingsPress?.invoke() }) {
                    Icon(Icons.Filled.Settings, contentDescription = "Settings", tint = ConsoleColors.TextPrimary)
                }
            }
        }
    }
}
