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

// Subagent rows moved to the shared protobuf schema (console.v1 from
// proto/console/v1): counts narrow to int32 and stay JSON numbers,
// activity args cross as raw JSON bytes. Start/activity/end EVENT
// frames are schema'd too (console.v1.SubagentStartEvent etc.) and
// stay flattened under the frame's type tag; Android reads subagent
// rows from GET /subagents, so the flat frames are covered by the
// golden fixtures instead of a decode path here.

/** Agent session SSE event. */
@Serializable
data class AgentSessionEvent(
    val type: String,
    val prompt: String? = null,
    val turnId: String? = null,
    // Per-token delta: raw until consumed below; the payload is
    // console.v1.ModelStreamPart, decoded with Moshi in ChatEvents
    // (same nested-proto pattern as todo items).
    val part: JsonElement? = null,
    // Turn-close snapshot: raw until consumed below; the payload is
    // console.v1.AgentAssistantMessage, decoded with Moshi in ChatEvents.
    val turn: JsonElement? = null,
    // Live tool payloads: raw until consumed below; the payloads are
    // console.v1.ToolCall/ToolResult (args/content as JSON bytes),
    // decoded with Moshi in ChatEvents like todo items.
    val calls: List<JsonElement>? = null,
    val request: JsonElement? = null,
    // contextUpdate payload: raw until consumed; console.v1.ContextSnapshot,
    // decoded with Moshi in ChatEvents.
    val context: JsonElement? = null,
    val result: JsonElement? = null,
    val results: List<JsonElement>? = null,
    // Wire TodoItems arrive here as raw JSON (Phase 4 will schema the
    // event frames); decoded with Moshi in ChatEvents.
    val items: List<JsonElement>? = null,
    val action: String? = null,
    val summary: String? = null,
    val originalMessageCount: Int? = null,
    val compactedMessageCount: Int? = null,
    val tokensBefore: Int? = null,
    val tokensAfter: Int? = null,
    val compactedMessages: List<AgentMessage>? = null,
    val title: String? = null,
    val error: EventError? = null,
    // Staged next-turn prompt: raw until the event stream migrates (Phase 4
    // schemas the frames); the payload is console.v1.QueuedPrompt, decoded
    // with Moshi where the queue screen needs it (see SessionQueueFixtureTest).
    val queuedPrompt: JsonElement? = null,
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
data class EventError(val message: String, val data: JsonElement? = null)

@Serializable
data class SseEventFrame(val event: String, val data: AgentSessionEvent)
