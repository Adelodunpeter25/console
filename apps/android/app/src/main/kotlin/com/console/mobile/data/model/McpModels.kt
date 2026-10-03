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

/**
 * One row of `GET /api/mcp/servers` — the canonical `ServerConfig` plus
 * live status. All fields defaulted: the row must decode even when the
 * server omits sections.
 */
@Serializable
data class McpServerEntry(
    val id: String,
    val label: String = "",
    val transport: String = "stdio",
    val url: String? = null,
    val command: String? = null,
    val args: List<String> = emptyList(),
    val env: Map<String, String> = emptyMap(),
    val auth: McpAuthConfig? = null,
    val enabled: Boolean = true,
    val status: String = "disconnected",
    val error: String? = null,
    val authUrl: String? = null,
    val toolCount: Int = 0,
    val tools: List<McpToolInfo> = emptyList(),
) {
    val displayName: String get() = label.ifBlank { id }
    val isConnected: Boolean get() = status == "connected"
    val isWorking: Boolean get() = status == "connecting" || status == "needs_auth"
    val needsAuth: Boolean get() = status == "needs_auth"
}

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
