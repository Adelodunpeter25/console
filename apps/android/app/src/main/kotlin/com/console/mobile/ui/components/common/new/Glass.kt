package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.ripple
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme

/**
 * Applies a solid floating surface effect:
 * 1. Clean drop shadow for floating elevation
 * 2. Solid dark surface background (no blur/translucency)
 * 3. Subtle hairline rim border
 */
fun Modifier.floatingSurface(
    shape: Shape = RoundedCornerShape(24.dp),
    backgroundColor: Color = Color(0xFF1E1E24),
    borderColor: Color = Color.White.copy(alpha = 0.08f),
    borderWidth: Dp = 1.dp,
    elevation: Dp = 4.dp,
): Modifier {
    return this
        .shadow(
            elevation = elevation,
            shape = shape,
            spotColor = Color.Black.copy(alpha = 0.40f),
            ambientColor = Color.Black.copy(alpha = 0.20f),
        )
        .clip(shape)
        .background(backgroundColor)
        .border(
            width = borderWidth,
            color = borderColor,
            shape = shape,
        )
}

/**
 * A floating circular action button with solid raised styling and subtle border.
 */
@Composable
fun FloatingGlassIconButton(
    icon: ImageVector,
    contentDescription: String?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: Dp = 46.dp,
    iconSize: Dp = 22.dp,
    tint: Color = NewTheme.TextPrimary,
    backgroundColor: Color = Color(0xFF1E1E24),
) {
    val interactionSource = remember { MutableInteractionSource() }
    Box(
        modifier = modifier
            .size(size)
            .floatingSurface(
                shape = CircleShape,
                backgroundColor = backgroundColor,
                borderColor = Color.White.copy(alpha = 0.08f),
                elevation = 4.dp,
            )
            .clickable(
                interactionSource = interactionSource,
                indication = ripple(bounded = true, radius = size / 2),
                onClick = onClick,
            ),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = contentDescription,
            tint = tint,
            modifier = Modifier.size(iconSize),
        )
    }
}
