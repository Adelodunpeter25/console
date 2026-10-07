package com.console.mobile.data.model

import com.console.mobile.data.api.ConsoleJson
import console.v1.AgentMessage as WireMessage
import console.v1.AssistantContentPart as WirePart
import kotlinx.serialization.json.JsonElement

/**
 * Wire-to-render conversion for conversation messages. The hand-written
 * message types in AgentModels are the transcript render models: the UI
 * constructs them optimistically, matches on them everywhere, and persists
 * them in ChatPersistence. They decode from the canonical oneof shape here —
 * never derive wire serializers.
 */
private fun bytesToJson(bytes: okio.ByteString): JsonElement? = try {
    ConsoleJson.parseToJsonElement(bytes.toByteArray().toString(Charsets.UTF_8))
} catch (_: Exception) {
    null
}

private fun console.v1.ImageAttachment.toUi(): ImagePart =
    ImagePart(data = data_, mimeType = mime_type)

internal fun console.v1.ToolCall.toUi(): ToolCall = ToolCall(
    id = id,
    name = name,
    arguments = bytesToJson(arguments),
    thoughtSignature = thought_signature,
)

internal fun console.v1.ToolResult.toUi(): ToolResult = ToolResult(
    toolCallId = tool_call_id,
    toolName = tool_name,
    content = bytesToJson(content),
    isError = is_error ?: false,
)

private fun console.v1.AssistantContentPart.toUi(): MessageContent? = when (val part = part) {
    is console.v1.AssistantContentPart.Part.Text ->
        TextPart(text = part.value.text, thoughtSignature = part.value.thought_signature)
    is console.v1.AssistantContentPart.Part.Thinking ->
        ThinkingPart(text = part.value.text)
    is console.v1.AssistantContentPart.Part.ToolCall -> ToolCallPart(call = part.value.toUi())
    is console.v1.AssistantContentPart.Part.Image ->
        ImagePart(data = part.value.data_, mimeType = part.value.mime_type)
    null -> null
}

fun WireMessage.toUi(): AgentMessage? = when (val event = message) {
    is console.v1.AgentMessage.Message.User -> UserMessage(
        id = id,
        createdAt = created_at,
        content = event.value.content,
        attachments = event.value.attachments.map { it.toUi() },
        contextFiles = event.value.context_files,
    )
    is console.v1.AgentMessage.Message.Assistant -> AssistantMessage(
        id = event.value.id.takeIf { it.isNotEmpty() },
        createdAt = created_at,
        content = event.value.content.mapNotNull { it.toUi() },
        stopReason = event.value.stop_reason.takeIf { it.isNotEmpty() },
    )
    is console.v1.AgentMessage.Message.ToolResult -> ToolResultMessage(
        id = id,
        createdAt = created_at,
        results = event.value.results.map { it.toUi() },
    )
    null -> null
}
