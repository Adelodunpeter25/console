package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
enum class SessionStatus(val value: String) {
    @kotlinx.serialization.SerialName("idle") Idle("idle"),
    @kotlinx.serialization.SerialName("working") Working("working"),
    @kotlinx.serialization.SerialName("done") Done("done"),
    @kotlinx.serialization.SerialName("needs_attention") NeedsAttention("needs_attention");

    companion object {
        fun fromValue(v: String?): SessionStatus = entries.firstOrNull { it.value == v } ?: Idle
    }
}

@Serializable
data class SessionHeader(
    val id: String,
    val title: String,
    val cwd: String,
    val projectId: String? = null,
    val modelId: String,
    val provider: String,
    val createdAt: Long,
    val updatedAt: Long,
    val messageCount: Int? = null,
    val status: SessionStatus? = null,
    val approvalMode: String? = null,
    val deletedAt: Long? = null,
)

@Serializable
data class SessionFileChange(
    val path: String,
    val status: String,
    val additions: Int = 0,
    val deletions: Int = 0,
    val turnIndex: Int = 0,
    val updatedAt: Long = 0,
)
