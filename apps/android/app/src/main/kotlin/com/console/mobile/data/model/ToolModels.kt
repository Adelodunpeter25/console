package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
enum class ToolTier { read, write, exec }

@Serializable
enum class ApprovalMode(val value: String) {
    @kotlinx.serialization.SerialName("always-ask") AlwaysAsk("always-ask"),
    @kotlinx.serialization.SerialName("accept-edits") AcceptEdits("accept-edits"),
    @kotlinx.serialization.SerialName("plan-mode") PlanMode("plan-mode"),
    @kotlinx.serialization.SerialName("full-access") FullAccess("full-access");

    companion object {
        fun fromValue(v: String?): ApprovalMode = entries.firstOrNull { it.value == v } ?: AlwaysAsk
    }
}

@Serializable
data class ApprovalModeOption(
    val value: ApprovalMode,
    val label: String,
    val description: String,
)

@Serializable
enum class ApprovalPolicy { allow, deny, prompt }

@Serializable
data class PermissionRequest(
    val requestId: String,
    val toolCallId: String,
    val toolName: String,
    val args: kotlinx.serialization.json.JsonElement? = null,
    val tier: ToolTier? = null,
    val reason: String? = null,
    val requiresUpgrade: Boolean = false,
)

@Serializable
data class ToolCall(
    val id: String,
    val name: String,
    val arguments: kotlinx.serialization.json.JsonElement? = null,
    val thoughtSignature: String? = null,
)

// ToolCallPreview moved to the shared protobuf schema
// (console.v1.ToolCallPreview): the stream preview carries id+name only;
// args arrive with the toolExecutionStart frame.


@Serializable
data class ToolResult(
    val toolCallId: String,
    val toolName: String? = null,
    val content: kotlinx.serialization.json.JsonElement? = null,
    val isError: Boolean = false,
)

class ToolError(message: String) : Exception(message)
