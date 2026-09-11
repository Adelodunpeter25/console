package com.console.mobile.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/**
 * Typography. Mirrors theme.fonts.mono* from styles/theme.ts.
 * JetBrainsMono font files should be placed in res/font/ and wired here
 * via Font(R.font.jetbrains_mono_*) when available. Until then we fall back
 * to FontFamily.Monospace so text still renders with mono metrics.
 */
val ConsoleMonoFamily: FontFamily = FontFamily.Monospace

val ConsoleTypography = Typography(
    // Display / headings map to theme text colors via Color — not font — so keep defaults.
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 14.sp,
        lineHeight = 20.sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontSize = 13.sp,
        lineHeight = 18.sp,
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 13.sp,
    ),
    // Mono — used by CodeViewer / terminal / VirtualizedCodeView equivalent
    bodySmall = TextStyle(
        fontFamily = ConsoleMonoFamily,
        fontSize = 12.sp,
        lineHeight = 18.sp,
    ),
)

object ConsoleDimens {
    // theme.roundness — dp already, map directly
    val RoundSm = 8
    val RoundMd = 12
    val RoundLg = 16
    // theme.spacing — dp
    val SpaceXs = 4
    val SpaceSm = 8
    val SpaceMd = 12
    val SpaceLg = 16
    val SpaceXl = 24
    val SpaceXxl = 32

    // Code viewer — mirrors VirtualizedCodeView constants
    val CodeLineHeight = 20
    const val CodeFontSizeSp = 11
    const val CodeGutterFontSizeSp = 10.5f
}
