package com.console.mobile.data.model

import kotlinx.serialization.Serializable

/**
 * Port of console-core types/settings.rs `ModelRoleMapping` — the harness
 * roles that resolve to a model instead of the session's chat model.
 * Each value is a `"provider/model"` reference, or null when the role is unset
 * (the server then falls back to the chat model).
 */
@Serializable
data class ModelRoleMapping(
    val vision: String? = null,
    val smol: String? = null,
) {
    /** Non-empty references keyed by role, for display and round-tripping. */
    fun toRefMap(): Map<String, String> = buildMap {
        vision?.takeIf { it.isNotBlank() }?.let { put(ROLE_VISION, it) }
        smol?.takeIf { it.isNotBlank() }?.let { put(ROLE_SMOL, it) }
    }
}

const val ROLE_VISION = "vision"
const val ROLE_SMOL = "smol"

/** Roles known to the server, in display order. */
val MODEL_ROLES = listOf(ROLE_VISION, ROLE_SMOL)

/** Response of `GET /api/settings`. */
@Serializable
data class ConsoleSettings(
    val modelRoles: ModelRoleMapping = ModelRoleMapping(),
)
