package com.console.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors

enum class PillButtonVariant { Filled, Outline }

/**
 * Pill-shaped action button (999dp corners) shared across the app — bottom-sheet
 * primary actions (e.g. environment "Save") and compact inline actions (e.g. account
 * "Login" / "Re-login"). Fully custom (not built on TextButton/Button) so content
 * padding is the only padding applied — no stacked default min-height.
 */
@Composable
fun PillButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    variant: PillButtonVariant = PillButtonVariant.Filled,
    enabled: Boolean = true,
    loading: Boolean = false,
    icon: ImageVector? = null,
    fullWidth: Boolean = false,
    horizontalPadding: androidx.compose.ui.unit.Dp = 14.dp,
    verticalPadding: androidx.compose.ui.unit.Dp = 8.dp,
) {
    val shape = RoundedCornerShape(999.dp)
    val contentColor = when {
        variant == PillButtonVariant.Filled && enabled -> Color.Black
        variant == PillButtonVariant.Filled -> Color.Black.copy(alpha = 0.4f)
        else -> ConsoleColors.TextPrimary
    }
    var m = modifier
    if (fullWidth) m = m.fillMaxWidth()
    m = m.clip(shape)
    m = when (variant) {
        PillButtonVariant.Filled -> m.background(if (enabled) Color.White else Color.White.copy(alpha = 0.4f))
        PillButtonVariant.Outline -> m.border(1.dp, ConsoleColors.Border, shape)
    }
    m = m.clickable(enabled = enabled && !loading, onClick = onClick)
        .padding(horizontal = horizontalPadding, vertical = verticalPadding)

    CompositionLocalProvider(LocalContentColor provides contentColor) {
        Row(modifier = m, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = if (fullWidth) Arrangement.Center else Arrangement.Start) {
            when {
                loading -> CircularProgressIndicator(color = contentColor, strokeWidth = 2.dp, modifier = Modifier.size(13.dp))
                icon != null -> Icon(icon, contentDescription = null, tint = contentColor, modifier = Modifier.size(13.dp))
            }
            if (loading || icon != null) Spacer(Modifier.size(6.dp))
            Text(text, color = contentColor, fontSize = 13.sp, fontWeight = FontWeight.Bold)
        }
    }
}
