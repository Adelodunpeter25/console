package com.console.mobile.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * Syntax-highlighting and diff colours only. Everything else lives in [NewTheme]
 * (ThemeNew.kt). These keep their own palette on purpose: they are tuned for
 * readability of code against a dark background, not part of the app's brand.
 */
object ConsoleColors {
    // Vitesse-dark / syntax — from services/highlighter.ts + TOKEN_STYLES
    // Kept here so Sora TextMate theme + Prism fallback share one palette.
    object Syntax {
        val Plain = Color(0xFFE4E4E7)
        val Keyword = Color(0xFFC084FC)
        val Builtin = Color(0xFF38BDF8)
        val ClassName = Color(0xFFFACC15)
        val Type = Color(0xFFFACC15)
        val String = Color(0xFF4ADE80)
        val Number = Color(0xFFFB923C)
        val Comment = Color(0xFF71717A)
        // Diff row tints. Alpha tints rather than solid fills so the syntax
        // colours underneath stay readable.
        val DiffAddedBg = Color(0x1A34D399)
        val DiffRemovedBg = Color(0x1AF87171)
    }
}
