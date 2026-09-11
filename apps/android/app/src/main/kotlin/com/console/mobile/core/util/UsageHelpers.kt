package com.console.mobile.core.util

import com.console.mobile.data.model.UsageLimit
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
    val resets = formatResetsAt(limit.window?.resetsAt, nowMs)
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
    val used = limit.amount.usedFraction
        ?: limit.amount.used?.let { it / 100.0 }
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
    if (limit.amount.usedFraction != null) return Math.round(limit.amount.usedFraction * 100).toInt()
    if (limit.amount.used != null) return Math.round(limit.amount.used).toInt()
    return null
}

fun getBarPercent(limit: UsageLimit): Double {
    if (limit.amount.usedFraction != null) return limit.amount.usedFraction.coerceIn(0.0, 1.0) * 100
    if (limit.amount.remainingFraction != null) return (1 - limit.amount.remainingFraction).coerceIn(0.0, 1.0) * 100
    return 0.0
}
