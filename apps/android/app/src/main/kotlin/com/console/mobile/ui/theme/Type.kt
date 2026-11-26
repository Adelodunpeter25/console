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
    displaySmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Bold,
        fontSize = 32.sp,
        lineHeight = 38.sp,
        letterSpacing = (-0.5).sp,
    ),
    headlineMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 26.sp,
        lineHeight = 32.sp,
        letterSpacing = (-0.3).sp,
    ),
    headlineSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 22.sp,
        lineHeight = 28.sp,
        letterSpacing = (-0.2).sp,
    ),
    titleLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 20.sp,
        lineHeight = 26.sp,
        letterSpacing = (-0.15).sp,
    ),
    titleMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 16.sp,
        lineHeight = 22.sp,
    ),
    titleSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 14.sp,
        lineHeight = 20.sp,
    ),
    bodyLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 15.sp,
        lineHeight = 22.sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Normal,
        fontSize = 13.sp,
        lineHeight = 18.sp,
    ),
    // Mono — used by CodeViewer / terminal / VirtualizedCodeView equivalent
    bodySmall = TextStyle(
        fontFamily = ConsoleMonoFamily,
        fontSize = 12.sp,
        lineHeight = 18.sp,
    ),
    labelLarge = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.SemiBold,
        fontSize = 13.sp,
        lineHeight = 18.sp,
    ),
    labelMedium = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 12.sp,
        lineHeight = 16.sp,
    ),
    labelSmall = TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = FontWeight.Medium,
        fontSize = 11.sp,
        lineHeight = 14.sp,
        letterSpacing = 0.2.sp,
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
