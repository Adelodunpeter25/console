package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class TodoItem(val id: Int, val content: String, val status: String)

object TodoStatus {
    const val PENDING = "pending"
    const val IN_PROGRESS = "in_progress"
    const val COMPLETED = "completed"
}

@Serializable
data class NotificationEvent(val type: String = "notification", val kind: String, val sessionId: String, val title: String, val subtitle: String = "", val body: String)

// Usage quota types moved to the shared protobuf schema (console.v1 from
// proto/console/v1): trimmed to what the UI renders, with unit/status as
// plain strings. resolveUsedFraction stays hand-written against the Wire
// type — the resolver fallback chain is behavior, not schema.

fun resolveUsedFraction(limit: console.v1.UsageLimit): Double? {
    val a = limit.amount ?: return null
    if (a.used_fraction != null) return a.used_fraction
    if (a.used != null && a.limit != null && a.limit > 0) return a.used / a.limit
    if (a.unit == "percent" && a.used != null) return a.used / 100.0
    if (a.remaining_fraction != null) return maxOf(0.0, 1 - a.remaining_fraction)
    return null
}
