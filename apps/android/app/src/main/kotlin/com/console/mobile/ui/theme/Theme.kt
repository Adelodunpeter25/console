package com.console.mobile.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/**
 * Console is dark-only (background #0a0a0b). We still expose a light hook
 * for previews. Dark scheme maps NewTheme into Material3 ColorScheme so
 * Scaffold/Card/BottomSheet pick up the right surfaces automatically.
 */
private val ConsoleDarkScheme = darkColorScheme(
    primary = NewTheme.Primary,
    onPrimary = NewTheme.OnPrimary,
    primaryContainer = NewTheme.Raised,
    onPrimaryContainer = NewTheme.TextPrimary,
    secondary = NewTheme.TextSecondary,
    onSecondary = NewTheme.TextPrimary,
    background = NewTheme.Background,
    onBackground = NewTheme.TextPrimary,
    surface = NewTheme.Card,
    onSurface = NewTheme.TextPrimary,
    surfaceVariant = NewTheme.Raised,
    onSurfaceVariant = NewTheme.TextSecondary,
    outline = NewTheme.TextGhost,
    outlineVariant = NewTheme.Divider,
    error = NewTheme.Danger,
    onError = Color.White,
    errorContainer = NewTheme.Danger.copy(alpha = 0.18f),
    scrim = Color.Black.copy(alpha = 0.45f),
)

@Composable
fun ConsoleTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    // Console is dark; ignore light for now but keep param for previews/tests.
    val scheme = ConsoleDarkScheme
    MaterialTheme(
        colorScheme = scheme,
        typography = ConsoleTypography,
        content = content,
    )
}
