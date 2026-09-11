package com.console.mobile.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class AskQuestionRequest(
    val requestId: String,
    val question: String,
    val options: List<String> = emptyList(),
    val isMultiSelect: Boolean = false,
    val skippable: Boolean = false,
    val batchId: String? = null,
)

@Serializable
data class SubagentStartEvent(
    val type: String = "subagentStart",
    val subagentId: String,
    val parentToolCallId: String,
    val name: String,
    val role: String,
    val prompt: String,
    val maxTurns: Int,
)

@Serializable
data class SubagentActivityEvent(
    val type: String = "subagentActivity",
    val subagentId: String,
    val turnIndex: Int,
    val toolCallId: String,
    val toolName: String,
    val args: JsonElement? = null,
    val status: String,
    val error: String? = null,
)

@Serializable
data class SubagentEndEvent(
    val type: String = "subagentEnd",
    val subagentId: String,
    val status: String,
    val summary: String? = null,
    val error: String? = null,
    val totalTurns: Int,
)

@Serializable
data class SubagentActivityItem(
    val turnIndex: Int,
    val toolCallId: String,
    val toolName: String,
    val summary: String? = null,
    val args: JsonElement? = null,
    val status: String,
    val error: String? = null,
)

@Serializable
data class SubagentInfo(
    val subagentId: String,
    val parentToolCallId: String,
    val name: String,
    val role: String,
    val prompt: String,
    val maxTurns: Int,
    val currentTurn: Int = 1,
    val status: String,
    val summary: String? = null,
    val error: String? = null,
    val activities: List<SubagentActivityItem> = emptyList(),
    val createdAt: Long? = null,
    val updatedAt: Long? = null,
)

/**
 * Agent session SSE event. Port of AgentSessionEvent union in packages/types.
 * `type` discriminates; only the relevant payload fields are set.
 */
@Serializable
data class AgentSessionEvent(
    val type: String,
    val prompt: String? = null,
    val turnId: String? = null,
    val part: StreamPart? = null,
    val turn: AssistantMessage? = null,
    val calls: List<ToolCall>? = null,
    val request: JsonElement? = null,
    val result: ToolResult? = null,
    val results: List<ToolResult>? = null,
    val items: List<TodoItem>? = null,
    val action: String? = null,
    val summary: String? = null,
    val originalMessageCount: Int? = null,
    val compactedMessageCount: Int? = null,
    val tokensBefore: Int? = null,
    val tokensAfter: Int? = null,
    val compactedMessages: List<AgentMessage>? = null,
    val title: String? = null,
    val error: EventError? = null,
    val queuedPrompt: QueuedPrompt? = null,
    val reason: String? = null,
    // subagent fields (flattened)
    val subagentId: String? = null,
    val parentToolCallId: String? = null,
    val name: String? = null,
    val role: String? = null,
    val maxTurns: Int? = null,
    val turnIndex: Int? = null,
    val toolCallId: String? = null,
    val toolName: String? = null,
    val args: JsonElement? = null,
    val status: String? = null,
    val totalTurns: Int? = null,
)

@Serializable
data class StreamPart(
    val text: String? = null,
    val thinking: String? = null,
    val toolCall: ToolCallPreview? = null,
)

@Serializable
data class EventError(val message: String, val data: JsonElement? = null)

@Serializable
data class SseEventFrame(val event: String, val data: AgentSessionEvent)
