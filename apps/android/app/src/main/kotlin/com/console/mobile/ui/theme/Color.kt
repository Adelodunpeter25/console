package com.console.mobile.ui.theme

import androidx.compose.ui.graphics.Color

/**
 * Port of apps/mobile/styles/theme.ts + global.css --color-* tokens.
 * Single source of truth remains global.css; this file mirrors it for Compose.
 * Regenerate intent: edit global.css then update here (or run theme:generate equivalent).
 */
object ConsoleColors {
    // Base — --color-screen / theme.colors.background
    val Background = Color(0xFF0A0A0B)
    val BackgroundAlt = Color(0xFF0A0A0B)
    val Surface = Color(0xFF16171A)          // --color-surface
    val SurfaceElevated = Color(0xFF1F2024)  // --color-surface-elevated
    val SurfaceCard = Color(0xFF1C1C1E)      // --color-surface-card
    val Card = Color(0xFF121316)             // --color-card
    val CardAlt = Color(0xFF18191C)          // --color-card-alt

    // Borders — rgba(255,255,255, alpha)
    val Border = Color.White.copy(alpha = 0.12f)
    val BorderSubtle = Color.White.copy(alpha = 0.06f)

    // Text — --color-foreground*
    val TextPrimary = Color(0xFFFFFFFF)
    val TextSecondary = Color(0xFFA1A1AA)
    val TextMuted = Color(0xFF71717A)
    val TextDark = Color(0xFF000000)

    // Primary / destructive
    val Primary = Color(0xFFFFFFFF)
    val Destructive = Color(0xFFF87171)
    val DestructivePressed = Color(0xFFDC2626)

    // Status — --color-status-*
    val StatusRunning = Color(0xFFFB923C)
    val StatusRunningBg = StatusRunning.copy(alpha = 0.10f)
    val StatusReady = Color(0xFF34D399)
    val StatusReadyBg = StatusReady.copy(alpha = 0.10f)
    val StatusAttention = Color(0xFFF87171)
    val StatusAttentionBg = StatusAttention.copy(alpha = 0.10f)
    val StatusIdle = Color(0xFFA1A1AA)
    val StatusIdleBg = StatusIdle.copy(alpha = 0.10f)

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
    }
}
