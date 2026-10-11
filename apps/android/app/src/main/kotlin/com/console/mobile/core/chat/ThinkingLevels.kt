package com.console.mobile.core.chat

/**
 * Thinking-level helpers. Nothing here knows which levels exist: the backend
 * sends each model's `supported_thinking_levels` and `default_thinking_level`,
 * and these functions only work out where the user is within that list.
 * (Desktop's click-to-cycle stepper does the same — see thinking_stepper.rs.)
 */

/** The level in force: the saved one if the model still offers it, else the model's default, else the first. */
fun effectiveThinkingLevel(supported: List<String>, saved: String?, default: String?): String? {
    if (supported.isEmpty()) return null
    return saved?.takeIf { it in supported }
        ?: default?.takeIf { it in supported }
        ?: supported.first()
}

/** The level after [current], wrapping around. Null when the model has no levels. */
fun nextThinkingLevel(supported: List<String>, current: String?): String? {
    if (supported.isEmpty()) return null
    val index = supported.indexOf(current).coerceAtLeast(0)
    return supported[(index + 1) % supported.size]
}

/**
 * Step label like desktop's "3/5". A leading "none" level counts as step 0 so
 * "off" reads "0/N" rather than shifting every other level up by one.
 */
fun thinkingStepLabel(supported: List<String>, current: String?): String? {
    val level = current?.takeIf { it in supported } ?: return null
    val index = supported.indexOf(level)
    return if (supported.first() == "none") "$index/${supported.size - 1}" else "${index + 1}/${supported.size}"
}

/**
 * Brain-icon color per level, ported from desktop's `thinking_level_color`
 * (`apps/desktop/crates/console-ui/src/common/thinking_stepper.rs`).
 * Mobile is dark-only, so only the dark variants are mapped; `none` and
 * unknown levels fall back to the muted ghost tone.
 *
 * Returns a raw `0xAARRGGBB` Long for Compose's sRGB `Color(Long)`
 * constructor — NOT a ULong, which `Color(ULong)` would misread as an
 * already-packed color and crash in `toArgb` (see `Color.getColorSpace`).
 */
fun thinkingLevelColorHex(level: String): Long = when (level) {
    "minimal" -> 0xFF2DD4BF // Mint / Teal
    "low" -> 0xFF38BDF8 // Sky Blue
    "medium" -> 0xFF818CF8 // Indigo
    "high" -> 0xFFC084FC // Violet / Purple
    "xhigh" -> 0xFFF472B6 // Fuchsia / Magenta
    "max" -> 0xFFFBBF24 // Warm Amber / Gold
    else -> 0xFF575757 // Off / unknown: muted ghost
}

/** Whole-number percent (the server sends 0..100) for the ring; null until known. */
fun contextPercent(usedTokens: Int, contextWindow: Int, percentUsed: Double): Int? {
    val pct = when {
        percentUsed > 0.0 -> percentUsed
        contextWindow > 0 -> usedTokens * 100.0 / contextWindow
        else -> return null
    }
    return pct.toInt().coerceIn(0, 100)
}
