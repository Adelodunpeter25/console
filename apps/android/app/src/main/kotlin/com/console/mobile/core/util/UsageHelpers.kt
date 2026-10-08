package com.console.mobile.core.util

import console.v1.UsageLimit
import com.console.mobile.data.model.resolveUsedFraction

fun formatResetsAt(resetsAt: Long?, nowMs: Long = System.currentTimeMillis()): String? {
    if (resetsAt == null || resetsAt == 0L) return null
    val diff = resetsAt - nowMs
    if (diff <= 0) return "resetting…"
    val hours = diff / (1000 * 60 * 60)
    val mins = (diff % (1000 * 60 * 60)) / (1000 * 60)
    if (hours >= 24) {
        val days = hours / 24
        return "resets in ${days}d ${hours % 24}h"
    }
    if (hours > 0) return "resets in ${hours}h ${mins}m"
    return "resets in ${mins}m"
}

fun formatWindowLabel(limit: UsageLimit, nowMs: Long = System.currentTimeMillis()): String {
    val windowLabel = limit.window?.label ?: limit.window?.id ?: "Quota"
    val resets = formatResetsAt(limit.window?.resets_at, nowMs)
    return if (resets != null) "$windowLabel · $resets" else windowLabel
}

fun usageStatusColor(status: String?): String = when (status) {
    "exhausted" -> "#f87171"
    "warning" -> "#fbbf24"
    "ok" -> "#34d399"
    else -> "#71717a"
}

fun statusForLimit(limit: UsageLimit): String? {
    if (!limit.status.isNullOrEmpty() && limit.status != "unknown") return limit.status
    val used = limit.amount?.used_fraction
        ?: limit.amount?.used?.let { it / 100.0 }
        ?: resolveUsedFraction(limit)
        ?: return limit.status
    return when {
        used >= 1 -> "exhausted"
        used >= 0.5 -> "warning"
        else -> "ok"
    }
}

fun colorForLimit(limit: UsageLimit): String = usageStatusColor(statusForLimit(limit))

fun getUsedPercent(limit: UsageLimit): Int? {
    limit.amount?.used_fraction?.let { return Math.round(it * 100).toInt() }
    limit.amount?.used?.let { return Math.round(it).toInt() }
    return null
}

fun getBarPercent(limit: UsageLimit): Double {
    limit.amount?.used_fraction?.let { return it.coerceIn(0.0, 1.0) * 100 }
    limit.amount?.remaining_fraction?.let { return (1 - it).coerceIn(0.0, 1.0) * 100 }
    return 0.0
}

// ---- Usage sheet formatting: a port of desktop's usage_panel.rs, so both clients
// read the same numbers the same way. Locale.US throughout: the default locale
// would print "1,5K" for a user whose decimal separator is a comma.

/** Severity of a meter, mapped to a colour by the UI. */
enum class UsageTone { Normal, Warning, Danger }

private fun fmt(pattern: String, vararg args: Any): String = String.format(java.util.Locale.US, pattern, *args)

/** Token counts as the desktop's context tile writes them: 842, 12.4k, 1.0M. */
fun formatTokens(value: Long): String {
    val f = value.toDouble()
    return when {
        f >= 1_000_000.0 -> fmt("%.1fM", f / 1_000_000.0)
        f >= 1_000.0 -> fmt("%.1fk", f / 1_000.0)
        else -> value.toString()
    }
}

fun formatContextUsage(usedTokens: Long, contextWindow: Long): String =
    "${formatTokens(usedTokens)}/${formatTokens(contextWindow)}"

/** Percent used for a limit, 0..100. Same fallback order as desktop's resolve_used_percent. */
fun usedPercent(limit: UsageLimit): Double =
    ((com.console.mobile.data.model.resolveUsedFraction(limit) ?: 0.0) * 100.0).coerceIn(0.0, 100.0)

fun formatUsageAmount(value: Double, unit: String): String = when (unit) {
    "minutes" -> {
        val hours = value / 60.0
        if (hours >= 1.0) fmt("%.1fh", hours) else fmt("%.0fm", value)
    }
    "tokens" -> when {
        value >= 1_000_000.0 -> fmt("%.1fM", value / 1_000_000.0)
        value >= 1_000.0 -> fmt("%.1fK", value / 1_000.0)
        else -> fmt("%.0f", value)
    }
    "percent" -> fmt("%.0f%%", value)
    "usd" -> fmt("$%.2f", value)
    else -> fmt("%.0f", value) // "requests" and anything the server adds later
}

/** The right-hand value on a limit row: "3.2h/5.0h", "812 used", or just "42%". */
fun formatUsageValue(limit: UsageLimit, percent: Double): String {
    val amount = limit.amount ?: return fmt("%.0f%%", percent)
    val used = amount.used
    val cap = amount.limit
    return when {
        used != null && cap != null && amount.unit != "percent" ->
            "${formatUsageAmount(used, amount.unit)}/${formatUsageAmount(cap, amount.unit)}"
        used != null && cap == null && amount.unit != "percent" -> "${formatUsageAmount(used, amount.unit)} used"
        else -> fmt("%.0f%%", percent)
    }
}

/** Limit severity: exhausted/95%+ is danger, warning/80%+ is a warning. */
fun limitTone(limit: UsageLimit, percent: Double): UsageTone = when {
    percent >= 95.0 || limit.status == "exhausted" -> UsageTone.Danger
    percent >= 80.0 || limit.status == "warning" -> UsageTone.Warning
    else -> UsageTone.Normal
}

/**
 * Context severity: danger at the server's compaction threshold, warning from 50%.
 * An unset threshold (0) would read as "always over", so it falls back to 90%.
 */
fun contextTone(percent: Double, thresholdRatio: Double): UsageTone {
    val threshold = if (thresholdRatio > 0.0) (thresholdRatio * 100.0).coerceIn(0.0, 100.0) else 90.0
    return when {
        percent >= threshold -> UsageTone.Danger
        percent >= 50.0 -> UsageTone.Warning
        else -> UsageTone.Normal
    }
}

/**
 * Footer ring colour. Desktop's ring (usage_meter.rs) turns amber at 80% and red
 * at 95%, a different rule from the context bar inside its panel; this follows
 * the ring so the two glyphs match.
 */
fun ringTone(percent: Double): UsageTone = when {
    percent >= 95.0 -> UsageTone.Danger
    percent >= 80.0 -> UsageTone.Warning
    else -> UsageTone.Normal
}

/** Reset text for a row: the server's own label when it sent one, else a countdown. */
fun usageResetLabel(limit: UsageLimit, nowMs: Long = System.currentTimeMillis()): String? =
    limit.window?.reset_label?.takeIf { it.isNotBlank() } ?: formatResetsAt(limit.window?.resets_at, nowMs)
