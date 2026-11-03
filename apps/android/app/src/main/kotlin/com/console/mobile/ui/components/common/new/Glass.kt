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
import dev.chrisbanes.haze.HazeState
import dev.chrisbanes.haze.HazeStyle
import dev.chrisbanes.haze.hazeEffect

/**
 * Ambient HazeState for sharing backdrop blur context between a scrolling transcript
 * and overlaid floating chrome (header island, floating composer dock).
 */
val LocalHazeState = compositionLocalOf<HazeState?> { null }

/**
 * Standard liquid glass specular rim brush: bright rim at the top where ambient
 * light catches the curve, fading softly toward the bottom edge.
 */
fun liquidGlassRimBrush(
    topAlpha: Float = 0.28f,
    midAlpha: Float = 0.10f,
    bottomAlpha: Float = 0.04f,
): Brush = Brush.verticalGradient(
    0.0f to Color.White.copy(alpha = topAlpha),
    0.5f to Color.White.copy(alpha = midAlpha),
    1.0f to Color.White.copy(alpha = bottomAlpha),
)

/**
 * Applies a true liquid glass effect:
 * 1. Hardware backdrop blur via Haze (when available)
 * 2. Translucent tinted backing surface
 * 3. Specular rim highlight border
 * 4. Soft atmospheric drop shadow
 */
fun Modifier.liquidGlass(
    hazeState: HazeState? = null,
    shape: Shape = RoundedCornerShape(26.dp),
    backgroundColor: Color = Color(0xFF16161C).copy(alpha = 0.72f),
    borderWidth: Dp = 1.dp,
    rimBrush: Brush = liquidGlassRimBrush(),
    blurRadius: Dp = 24.dp,
    elevation: Dp = 6.dp,
): Modifier {
    val hazeMod = if (hazeState != null) {
        Modifier.hazeEffect(
            state = hazeState,
            style = HazeStyle(
                backgroundColor = backgroundColor,
                tints = emptyList(),
                blurRadius = blurRadius,
            ),
        )
    } else {
        Modifier.background(backgroundColor)
    }

    return this
        .shadow(
            elevation = elevation,
            shape = shape,
            spotColor = Color.Black.copy(alpha = 0.45f),
            ambientColor = Color.Black.copy(alpha = 0.25f),
        )
        .then(hazeMod)
        .clip(shape)
        .border(
            width = borderWidth,
            brush = rimBrush,
            shape = shape,
        )
}

/**
 * A floating circular action button styled with liquid glass translucency,
 * backdrop blur, and a specular rim.
 */
@Composable
fun FloatingGlassIconButton(
    icon: ImageVector,
    contentDescription: String?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    size: Dp = 42.dp,
    iconSize: Dp = 20.dp,
    tint: Color = NewTheme.TextPrimary,
    hazeState: HazeState? = LocalHazeState.current,
    backgroundColor: Color = Color(0xFF1A1A22).copy(alpha = 0.75f),
) {
    val interactionSource = remember { MutableInteractionSource() }
    Box(
        modifier = modifier
            .size(size)
            .liquidGlass(
                hazeState = hazeState,
                shape = CircleShape,
                backgroundColor = backgroundColor,
                elevation = 6.dp,
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
