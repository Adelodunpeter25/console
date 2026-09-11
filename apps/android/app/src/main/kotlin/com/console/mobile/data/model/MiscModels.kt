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
data class NotificationEvent(val type: String = "notification", val kind: String, val sessionId: String, val title: String, val body: String)

@Serializable
data class UsageWindow(val id: String, val label: String, val durationMs: Long? = null, val resetsAt: Long? = null, val resetLabel: String? = null)

@Serializable
data class UsageAmount(
    val used: Double? = null,
    val limit: Double? = null,
    val remaining: Double? = null,
    val usedFraction: Double? = null,
    val remainingFraction: Double? = null,
    val unit: String,
)

@Serializable
data class UsageScope(
    val provider: String,
    val accountId: String? = null,
    val projectId: String? = null,
    val orgId: String? = null,
    val modelId: String? = null,
    val tier: String? = null,
    val windowId: String? = null,
    val shared: Boolean? = null,
)

@Serializable
data class UsageLimit(
    val id: String,
    val label: String,
    val scope: UsageScope,
    val window: UsageWindow? = null,
    val amount: UsageAmount,
    val status: String? = null,
    val notes: List<String> = emptyList(),
)

@Serializable
data class UsageReport(
    val provider: String,
    val fetchedAt: Long,
    val limits: List<UsageLimit> = emptyList(),
    val resetCredits: UsageResetCredits? = null,
    val notes: List<String> = emptyList(),
)

@Serializable
data class UsageResetCredits(val availableCount: Int, val credits: List<UsageResetCreditDetail> = emptyList())

@Serializable
data class UsageResetCreditDetail(val grantedAt: String? = null, val expiresAt: String? = null, val status: String? = null)

fun resolveUsedFraction(limit: UsageLimit): Double? {
    val a = limit.amount
    if (a.usedFraction != null) return a.usedFraction
    if (a.used != null && a.limit != null && a.limit > 0) return a.used / a.limit
    if (a.unit == "percent" && a.used != null) return a.used / 100.0
    if (a.remainingFraction != null) return maxOf(0.0, 1 - a.remainingFraction)
    return null
}
