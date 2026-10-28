package com.console.mobile.data.model

import com.console.mobile.SharedNative
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
    // Server-classified presentation fields (proto console.v1.ToolCall):
    // label, kind flags and icon key are derived at the wire boundary so
    // clients render instead of parsing names. Absent in old stored
    // sessions, where SharedNative's native tables fill in.
    val label: String? = null,
    val kindFlags: Int? = null,
    val icon: String? = null,
) {
    private fun flag(bit: Int) = kindFlags?.let { it and bit != 0 } ?: false

    val isEdit: Boolean get() = flag(1) || (kindFlags == null && SharedNative.kind(name).edit)
    val isWrite: Boolean get() = flag(2) || (kindFlags == null && SharedNative.kind(name).write)
    val isRead: Boolean get() = flag(4) || (kindFlags == null && SharedNative.kind(name).read)
    val isFileTarget: Boolean get() = flag(8) || (kindFlags == null && SharedNative.kind(name).fileTarget)
    val isSubagent: Boolean get() = flag(16) || (kindFlags == null && SharedNative.kind(name).subagent)

    /** Display label: the server's when present, else the native table. */
    val displayLabel: String get() = label ?: SharedNative.label(name)
}

// ToolCallPreview moved to the shared protobuf schema
// (console.v1.ToolCallPreview): the stream preview carries id+name only;
// args arrive with the toolExecutionStart frame.


@Serializable
data class ToolResult(
    val toolCallId: String,
    val toolName: String? = null,
    val content: kotlinx.serialization.json.JsonElement? = null,
    val isError: Boolean = false,
    /** Client-rendered artifact (see feature/chat/visual). Never sent to a provider. */
    val visual: VisualArtifact? = null,
)

/** A versioned visual spec riding on a tool result; the spec stays raw JSON until rendered. */
@Serializable
data class VisualArtifact(
    val version: Int,
    val spec: kotlinx.serialization.json.JsonElement,
)

class ToolError(message: String) : Exception(message)
