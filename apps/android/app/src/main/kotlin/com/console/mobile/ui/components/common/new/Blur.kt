package com.console.mobile.ui.components.common.new

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.blur
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme

/**
 * A shared progressive/frosted blur header container for screens whose content scrolls
 * underneath the header bar.
 *
 * Places a background blur layer and gradient scrim behind the sharp [content].
 */
@Composable
fun BlurredHeaderContainer(
    modifier: Modifier = Modifier,
    backgroundColor: Color = NewTheme.Background,
    blurRadius: Dp = 20.dp,
    height: Dp = PageHeaderHeight,
    content: @Composable () -> Unit,
) {
    Box(modifier = modifier.fillMaxWidth().height(height)) {
        Box(
            modifier = Modifier
                .matchParentSize()
                .then(
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                        Modifier.blur(blurRadius)
                    } else {
                        Modifier
                    }
                )
                .background(
                    Brush.verticalGradient(
                        0.0f to backgroundColor.copy(alpha = 0.95f),
                        0.75f to backgroundColor.copy(alpha = 0.85f),
                        1.0f to backgroundColor.copy(alpha = 0.65f),
                    )
                ),
        )
        content()
    }
}

/**
 * A shared frosted/progressive blur container for bottom strips (such as the chat composer).
 *
 * Renders a bottom-anchored blur scrim with a top-to-bottom progressive fade:
 * completely transparent at the very top (so messages smoothly dissolve as they enter)
 * and rich dark background over the bottom interactive controls.
 */
@Composable
fun BlurredBottomBar(
    modifier: Modifier = Modifier,
    backgroundColor: Color = NewTheme.Background,
    blurRadius: Dp = 20.dp,
    content: @Composable BoxScope.() -> Unit,
) {
    Box(modifier = modifier.fillMaxWidth()) {
        Box(
            modifier = Modifier
                .matchParentSize()
                .then(
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                        Modifier.blur(blurRadius)
                    } else {
                        Modifier
                    }
                )
                .background(
                    Brush.verticalGradient(
                        0.0f to backgroundColor.copy(alpha = 0.70f),
                        0.35f to backgroundColor.copy(alpha = 0.92f),
                        1.0f to backgroundColor,
                    )
                ),
        )
        content()
    }
}
