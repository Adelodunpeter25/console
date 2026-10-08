package com.console.mobile.data.model

import kotlinx.serialization.Serializable

/** Mirrors the server's `AuthConfig` (`apps/server-go/internal/services/mcp`). */
@Serializable
data class McpAuthConfig(
    val type: String = "none",
    val tokenRef: String? = null,
)

/** Mirrors the server's `RemoteTool` (name + description only; schema omitted). */
@Serializable
data class McpToolInfo(
    val name: String = "",
    val description: String = "",
)

// McpServerEntry moved to the shared protobuf schema (console.v1.McpServerStatus
// from proto/console/v1/mcp.proto): the list row is flat — config keys plus
// live status — with transport/enabled always set and timestamps as strings.
// McpSavePayload stays hand-shaped: the save request keeps the dual
// name/auth_type spellings, enabled null-vs-absent, and the static token.

/** Body for `POST /api/mcp/servers` and `PUT /api/mcp/servers/:id`. */
@Serializable
data class McpSavePayload(
    val id: String? = null,
    val label: String,
    val transport: String,
    val url: String? = null,
    val command: String? = null,
    val args: List<String> = emptyList(),
    val env: Map<String, String> = emptyMap(),
    val auth: McpAuthConfig? = null,
    /** Static-token secret; stored server-side, never echoed back. */
    val token: String? = null,
    val enabled: Boolean = true,
)

/** Body for `POST /api/mcp/servers/:id/oauth/callback`. */
@Serializable
data class McpOAuthCallbackPayload(
    val state: String,
    val code: String? = null,
    val error: String? = null,
    val iss: String? = null,
)

// Render conveniences over the Wire row (console.v1.McpServerStatus): the
// hand class carried these as members; extensions keep every call site
// unchanged now that the row is schema'd.
val console.v1.McpServerStatus.displayName: String get() = label.ifBlank { id }
val console.v1.McpServerStatus.isConnected: Boolean get() = status == "connected"
val console.v1.McpServerStatus.isWorking: Boolean get() = status == "connecting" || status == "needs_auth"
val console.v1.McpServerStatus.needsAuth: Boolean get() = status == "needs_auth"
