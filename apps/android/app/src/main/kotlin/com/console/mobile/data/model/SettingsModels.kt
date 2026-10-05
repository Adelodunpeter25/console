package com.console.mobile.data.model

/**
 * Roles known to the server, in display order.
 *
 * Wire types own the wire now (console.v1 from proto/console/v1): the
 * hand-written ModelRoleMapping/ConsoleSettings were deleted in favor of
 * the generated ones. The PATCH *request* body stays hand-built in
 * OkHttpConsoleApi — a null role clears it while a missing key leaves it
 * untouched, which no proto3 map value can express.
 */
const val ROLE_VISION = "vision"
const val ROLE_SMOL = "smol"

/** Roles known to the server, in display order. */
val MODEL_ROLES = listOf(ROLE_VISION, ROLE_SMOL)

/** Non-empty references keyed by role, for display and round-tripping. */
fun console.v1.ModelRoleMapping.toRefMap(): Map<String, String> = buildMap {
    vision?.takeIf { it.isNotBlank() }?.let { put(ROLE_VISION, it) }
    smol?.takeIf { it.isNotBlank() }?.let { put(ROLE_SMOL, it) }
}
