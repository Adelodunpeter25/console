package com.console.mobile.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.code.JetBrainsMono

/**
 * Typography. Mirrors theme.fonts.mono* from styles/theme.ts.
 * JetBrains Mono now ships in res/font (copied from the desktop), so Compose
 * text and the Sora code viewer render the same glyphs.
 */
val ConsoleMonoFamily: FontFamily = JetBrainsMono

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

    // Code viewer — mirrors VirtualizedCodeView constants.
    // CodeLineHeight must match the viewer's painted line height: callers size
    // containers and gutters from it, and Sora derives its line height from the
    // text size, so these two move together.
    val CodeLineHeight = 24
    const val CodeFontSizeSp = 13f
    const val CodeGutterFontSizeSp = 12f
}
