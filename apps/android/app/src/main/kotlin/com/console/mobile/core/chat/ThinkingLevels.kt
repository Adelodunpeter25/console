package com.console.mobile.core.chat

import com.console.mobile.data.model.ThinkingLevels

/** A thinking level as the user sees it. [value] is what goes on the wire. */
data class ThinkingOption(val value: String, val label: String, val description: String)

private val KNOWN = listOf(
    ThinkingOption(ThinkingLevels.NONE, "Off", "No extended thinking"),
    ThinkingOption(ThinkingLevels.MINIMAL, "Minimal", "Quickest answers"),
    ThinkingOption(ThinkingLevels.LOW, "Fast", "Fastest"),
    ThinkingOption(ThinkingLevels.MEDIUM, "Balanced", "Balanced"),
    ThinkingOption(ThinkingLevels.HIGH, "Deep", "Thorough"),
    ThinkingOption(ThinkingLevels.XHIGH, "Extra deep", "Very thorough"),
    ThinkingOption(ThinkingLevels.MAX, "Max", "Maximum depth"),
)

/**
 * Options for a model, in the server's declared order. Raw enum strings never
 * reach the UI: an unknown level is kept (so a new server value still works)
 * but gets a capitalised label rather than being dropped.
 */
fun thinkingOptions(supported: List<String>): List<ThinkingOption> =
    supported.map { raw ->
        KNOWN.firstOrNull { it.value == raw }
            ?: ThinkingOption(raw, raw.replaceFirstChar { it.uppercase() }, "")
    }

/** Label for the chip; null hides the chip (model reports no levels). */
fun thinkingChipLabel(supported: List<String>, current: String?, default: String?): String? {
    if (supported.isEmpty()) return null
    val level = current?.takeIf { it in supported } ?: default?.takeIf { it in supported } ?: supported.first()
    return thinkingOptions(listOf(level)).first().label
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
