package com.console.mobile.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/**
 * Console is dark-only (background #0a0a0b). We still expose a light hook
 * for previews. Dark scheme maps ConsoleColors into Material3 ColorScheme so
 * Scaffold/Card/BottomSheet pick up the right surfaces automatically.
 */
private val ConsoleDarkScheme = darkColorScheme(
    primary = ConsoleColors.Primary,
    onPrimary = ConsoleColors.TextDark,
    primaryContainer = ConsoleColors.SurfaceElevated,
    onPrimaryContainer = ConsoleColors.TextPrimary,
    secondary = ConsoleColors.TextSecondary,
    onSecondary = ConsoleColors.TextPrimary,
    background = ConsoleColors.Background,
    onBackground = ConsoleColors.TextPrimary,
    surface = ConsoleColors.Surface,
    onSurface = ConsoleColors.TextPrimary,
    surfaceVariant = ConsoleColors.SurfaceElevated,
    onSurfaceVariant = ConsoleColors.TextSecondary,
    outline = ConsoleColors.Border,
    outlineVariant = ConsoleColors.BorderSubtle,
    error = ConsoleColors.Destructive,
    onError = Color.White,
    errorContainer = ConsoleColors.Destructive.copy(alpha = 0.18f),
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
