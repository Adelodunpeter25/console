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

private fun console.v1.BrowserAnnotation.toUi(): BrowserAnnotation = BrowserAnnotation(
    id = id,
    selector = selector,
    componentName = component_name,
    sourceLocation = source_location,
    userComment = user_comment,
    screenshotBase64 = screenshot_base64,
)

internal fun console.v1.ToolCall.toUi(): ToolCall = ToolCall(
    id = id,
    name = name,
    arguments = bytesToJson(arguments),
    thoughtSignature = thought_signature,
    // The server classifies every call (label, kind flags, icon key). They must
    // be carried across: the rows render from these and fall back to the raw
    // name and a generic icon without them.
    label = label,
    kindFlags = kind_flags,
    icon = icon,
)

internal fun console.v1.ToolResult.toUi(): ToolResult = ToolResult(
    toolCallId = tool_call_id,
    toolName = tool_name,
    content = bytesToJson(content),
    isError = is_error ?: false,
    // A malformed spec drops only the artifact; the summary content stays.
    visual = visual?.let { v -> bytesToJson(v.spec)?.let { VisualArtifact(v.version, it) } },
    html = html?.takeIf { it.path.isNotEmpty() }?.let { h ->
        HtmlArtifact(
            path = h.path,
            title = h.title,
            height = h.height,
            snapshotBase64 = h.snapshot?.takeIf { it.size > 0 }?.base64(),
        )
    },
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

internal fun console.v1.AgentAssistantMessage.toUi(): AssistantMessage = AssistantMessage(
    id = id.takeIf { it.isNotEmpty() },
    content = content.mapNotNull { it.toUi() },
    stopReason = stop_reason.takeIf { it.isNotEmpty() },
    tokPerSec = tok_per_sec,
)

fun WireMessage.toUi(): AgentMessage? = when (val event = message) {
    is console.v1.AgentMessage.Message.User -> UserMessage(
        id = id,
        createdAt = created_at,
        content = event.value.content,
        attachments = event.value.attachments.map { it.toUi() },
        annotations = event.value.annotations.map { it.toUi() },
        contextFiles = event.value.context_files,
        approvalMode = event.value.approval_mode,
        source = event.value.source,
    )
    is console.v1.AgentMessage.Message.Assistant -> AssistantMessage(
        id = event.value.id.takeIf { it.isNotEmpty() },
        createdAt = created_at,
        content = event.value.content.mapNotNull { it.toUi() },
        stopReason = event.value.stop_reason.takeIf { it.isNotEmpty() },
        tokPerSec = event.value.tok_per_sec,
    )
    is console.v1.AgentMessage.Message.ToolResult -> ToolResultMessage(
        id = id,
        createdAt = created_at,
        results = event.value.results.map { it.toUi() },
    )
    null -> null
}

private fun console.v1.RunStatus.toUi(): com.console.mobile.core.chat.RunStatus = when (this) {
    console.v1.RunStatus.RUN_STATUS_WORKING -> com.console.mobile.core.chat.RunStatus.Working
    console.v1.RunStatus.RUN_STATUS_ABORTED -> com.console.mobile.core.chat.RunStatus.Aborted
    console.v1.RunStatus.RUN_STATUS_ERROR -> com.console.mobile.core.chat.RunStatus.Failed
    else -> com.console.mobile.core.chat.RunStatus.Completed
}

internal fun console.v1.RunActivity.toUi(wakeLabel: String? = null): com.console.mobile.core.chat.RunActivityState =
    com.console.mobile.core.chat.RunActivityState(
        runId = run_id,
        startedAt = started_at,
        elapsedMs = elapsed_ms,
        status = status.toUi(),
        dividerLabel = wakeLabel,
        events = events.mapNotNull { ev ->
            when (val e = ev.event) {
                is console.v1.RunActivityEvent.Event.Thinking ->
                    com.console.mobile.core.chat.ActivityEvent.Thinking(ev.id, e.value)
                is console.v1.RunActivityEvent.Event.Text ->
                    com.console.mobile.core.chat.ActivityEvent.Text(ev.id, e.value)
                is console.v1.RunActivityEvent.Event.ToolCall -> {
                    val call = e.value.call?.toUi() ?: return@mapNotNull null
                    val res = e.value.result?.toUi()?.let { r ->
                        val size = e.value.result_size
                        val msgId = e.value.result_message_id.takeIf { it.isNotEmpty() }
                        if (size > 0 && msgId != null) r.copy(omittedSize = size, resultMessageId = msgId) else r
                    }
                    com.console.mobile.core.chat.ActivityEvent.ToolCallEvent(ev.id, call, res)
                }
                null -> null
            }
        },
    )

fun console.v1.TranscriptTurn.toUi(): com.console.mobile.core.chat.TranscriptTurn {
    val user = user_message?.toUi() as? UserMessage
    val resps = responses.mapNotNull { it.toUi() as? AssistantMessage }
    return com.console.mobile.core.chat.TranscriptTurn(
        id = id,
        createdAt = created_at,
        userMessage = user,
        wakeLabel = wake_label,
        run = run?.toUi(wake_label),
        responses = resps,
    )
}
