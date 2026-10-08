package com.console.mobile.ui.theme

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import com.console.mobile.core.util.UsageTone

/**
 * The new look, in one place. Screens being migrated read their colours and
 * shapes from here instead of hard-coding hex values or reaching into
 * [ConsoleColors]; once everything has moved, ConsoleColors can be retired.
 *
 * Brand and status colours are the desktop's dark theme (`theme/mod.rs`), so the
 * two clients agree. Surfaces and text keep the mobile values they have today.
 */
object NewTheme {
    // ---- Surfaces
    val Background = Color(0xFF0A0A0B)
    /** Grouped cards: raised a step above the background, no border. */
    val Card = Color(0xFF1C1C1E)
    /** Inputs sitting on top of a card or the background. */
    val Field = Color(0xFF121316)
    val Divider = Color.White.copy(alpha = 0.06f)

    // ---- Text
    val TextPrimary = Color(0xFFFFFFFF)
    val TextSecondary = Color(0xFFA1A1AA)
    val TextMuted = Color(0xFF71717A)
    /** Placeholder dots and anything that should barely register (desktop text_ghost). */
    val TextGhost = Color(0xFF575757)

    // ---- Brand
    /** Desktop's accent, the orange-brown (theme.accent, dark). Category headings, links. */
    val Accent = Color(0xFFE2795B)

    // ---- Status (desktop success / warning / danger / gauge)
    val Success = Color(0xFF62C987)
    val Warning = Color(0xFFE0B36A)
    val Danger = Color(0xFFE2726A)
    /** Meter blue (theme.gauge): a healthy bar or ring. */
    val Gauge = Color(0xFF3B82F6)

    // ---- Shapes
    val CardRadius = 26.dp
    val FieldRadius = 16.dp
    val ChipRadius = 12.dp
}

/** Colour for a usage meter of this severity. */
fun UsageTone.color(): Color = when (this) {
    UsageTone.Normal -> NewTheme.Gauge
    UsageTone.Warning -> NewTheme.Warning
    UsageTone.Danger -> NewTheme.Danger
}
