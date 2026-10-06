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

// SessionHeader moved to the shared protobuf schema (console.v1 from
// proto/console/v1): timestamps arrive as protojson strings. SessionStatus
// stays hand-written: UI vocabulary with an Idle fallback, mapped with
// fromValue at the repository boundary.


@Serializable
data class SessionFileChange(
    val path: String,
    val status: String,
    val additions: Int = 0,
    val deletions: Int = 0,
    val turnIndex: Int = 0,
    val updatedAt: Long = 0,
)
