package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors

/**
 * Port of components/layout/screen-header.tsx.
 * Title (22sp bold) + optional subtitle, both centered on the screen, with
 * back button left and settings + headerActions right. Box-based so the title
 * stays on the true center even when the side slots differ in width; the
 * center column keeps 56dp of side clearance so long titles ellipsize
 * instead of sliding under the buttons.
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
    Box(
        modifier = modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp),
    ) {
        if (onBack != null) {
            Surface(
                shape = CircleShape,
                color = ConsoleColors.Card,
                border = BorderStroke(1.dp, ConsoleColors.Border),
                modifier = Modifier.align(Alignment.CenterStart).size(40.dp),
            ) {
                IconButton(onClick = onBack) {
                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", tint = ConsoleColors.TextPrimary)
                }
            }
        }
        Column(
            modifier = Modifier.align(Alignment.Center).fillMaxWidth().padding(horizontal = 56.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                text = title,
                color = ConsoleColors.TextPrimary,
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold,
                textAlign = TextAlign.Center,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    color = ConsoleColors.TextSecondary,
                    fontSize = 12.sp,
                    textAlign = TextAlign.Center,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        Row(
            modifier = Modifier.align(Alignment.CenterEnd),
            verticalAlignment = Alignment.CenterVertically,
        ) {
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
}
