package com.console.mobile.core.util

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** Port of apps/mobile/utils/time.ts */
fun formatMessageTime(dateInput: Any?): String {
    if (dateInput == null) return ""
    val ms = when (dateInput) {
        is Number -> dateInput.toLong()
        is String -> dateInput.toLongOrNull() ?: try { Date.parse(dateInput) } catch (_: Exception) { return "" }
        else -> return ""
    }
    return try {
        SimpleDateFormat("h:mm a", Locale.US).format(Date(ms))
    } catch (_: Exception) {
        ""
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
