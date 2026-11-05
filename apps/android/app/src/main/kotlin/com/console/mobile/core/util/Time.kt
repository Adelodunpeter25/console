package com.console.mobile.core.util

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

fun formatMessageTime(dateInput: Any?): String {
    if (dateInput == null) return ""
    val ms = when (dateInput) {
        is Number -> dateInput.toLong()
        is String -> dateInput.toLongOrNull() ?: try { java.time.OffsetDateTime.parse(dateInput).toInstant().toEpochMilli() } catch (_: Exception) { return "" }
        else -> return ""
    }
    return try {
        SimpleDateFormat("h:mm a", Locale.US).format(Date(ms))
    } catch (_: Exception) {
        ""
    }
}

/**
 * Age label for the prompt list, same rules as the desktop's `format_time_ago`:
 * "just now", then one coarse unit ("5m ago", "3h ago", "2d ago"), and a bare
 * date like "Oct 29" once older than a week. [timestampMs] is Unix milliseconds.
 */
fun formatTimeAgo(timestampMs: Long, nowMs: Long = System.currentTimeMillis()): String {
    val seconds = ((nowMs - timestampMs) / 1000).coerceAtLeast(0)
    return when {
        seconds < 60 -> "just now"
        seconds < 3_600 -> "${seconds / 60}m ago"
        seconds < 86_400 -> "${seconds / 3_600}h ago"
        seconds < 604_800 -> "${seconds / 86_400}d ago"
        else -> SimpleDateFormat("MMM d", Locale.US).format(Date(timestampMs))
    }
}

fun formatRelativeTime(dateInput: Any?, nowMs: Long = System.currentTimeMillis()): String {
    if (dateInput == null) return ""
    val ms = when (dateInput) {
        is Number -> dateInput.toLong()
        is String -> dateInput.toLongOrNull() ?: return ""
        else -> return ""
    }
    val diffSec = (nowMs - ms) / 1000
    if (diffSec < 0) return ""
    if (diffSec < 60) return "just now"
    val diffMin = diffSec / 60
    if (diffMin < 60) return "${diffMin}m ago"
    val diffHr = diffMin / 60
    if (diffHr < 24) return "${diffHr}h ago"
    return "${diffHr / 24}d ago"
}
