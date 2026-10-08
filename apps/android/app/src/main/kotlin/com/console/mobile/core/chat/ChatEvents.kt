package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.UserMessage
import com.console.mobile.data.model.ToolResultMessage
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentAssistantMessage as WireAssistantMessage
import console.v1.ContextSnapshot
import console.v1.ModelStreamPart
import console.v1.TodoItem
import console.v1.ToolCall as WireToolCall
import console.v1.ToolResult as WireToolResult
import com.console.mobile.data.model.toUi
import java.util.UUID

private val todoJson = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(TodoItem::class.java)
private val partJson = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(ModelStreamPart::class.java)
private val wireMoshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
private val toolCallJson = wireMoshi.adapter(WireToolCall::class.java)
private val toolResultJson = wireMoshi.adapter(WireToolResult::class.java)
private val contextJson = wireMoshi.adapter(ContextSnapshot::class.java)
private val turnJson = wireMoshi.adapter(WireAssistantMessage::class.java)

private fun decodeToolCall(raw: kotlinx.serialization.json.JsonElement): com.console.mobile.data.model.ToolCall? =
    runCatching { toolCallJson.fromJson(raw.toString()) }.getOrNull()?.toUi()

private fun decodeToolResult(raw: kotlinx.serialization.json.JsonElement): com.console.mobile.data.model.ToolResult? =
    runCatching { toolResultJson.fromJson(raw.toString()) }.getOrNull()?.toUi()


fun newMessageId(): String = try {
    UUID.randomUUID().toString()
} catch (_: Exception) {
    "msg-${System.currentTimeMillis()}-${(Math.random() * 1e6).toInt()}"
}

fun ensureMessageIds(messages: List<AgentMessage>): List<AgentMessage> {
    var changed = false
    val out = messages.map { m ->
        if (m.id != null) m else {
            changed = true
            when (m) {
                is UserMessage -> m.copy(id = newMessageId())
                is AssistantMessage -> m.copy(id = newMessageId())
                is ToolResultMessage -> m.copy(id = newMessageId())
            }
        }
    }
    return if (changed) out else messages
}

private fun updateLatestRun(
    session: ChatSessionState,
    update: (RunActivityState) -> RunActivityState,
): ChatSessionState {
    if (session.runs.isEmpty()) return session
    val runs = session.runs.toMutableList()
    runs[runs.lastIndex] = update(runs.last())
    return session.copy(runs = runs)
}

private fun setToolCallResult(events: List<ActivityEvent>, result: ToolResult): List<ActivityEvent> =
    events.map { e ->
        if (e is ActivityEvent.ToolCallEvent && e.call.id == result.toolCallId) e.copy(result = result) else e
    }

private fun finalizePendingToolCalls(events: List<ActivityEvent>): List<ActivityEvent> =
    events.map { e ->
        if (e is ActivityEvent.ToolCallEvent && e.result == null) {
            e.copy(result = ToolResult(toolCallId = e.call.id, toolName = e.call.name, content = null, isError = true))
        } else e
    }

/**
 * Clears the streaming buffers, first flushing whatever partial assistant text is
 * already on screen into `messages` as a completed message.
 *
 * Without this, tapping Stop — or a run erroring — silently deletes the whole
 * in-progress reply, because the streamed text only ever lived in
 * `streamingText`/`streamingThinking` until a `modelStreamEnd` carried the real
 * turn message.
 */
fun commitStreamingBuffer(session: ChatSessionState): ChatSessionState {
    if (session.streamingText.isEmpty() && session.streamingThinking.isEmpty()) {
        return session.copy(streamingText = "", streamingThinking = "")
    }
    val parts = buildList {
        if (session.streamingThinking.isNotEmpty()) add(ThinkingPart(session.streamingThinking))
        if (session.streamingText.isNotEmpty()) add(TextPart(session.streamingText))
    }
    return session.copy(
        messages = session.messages + AssistantMessage(
            id = newMessageId(),
            createdAt = System.currentTimeMillis(),
            content = parts,
        ),
        streamingText = "",
        streamingThinking = "",
    )
}

fun applyChatEvent(session: ChatSessionState, event: AgentSessionEvent): ChatSessionState {
    return when (event.type) {
        "modelStreamPart" -> {
            val part = event.part?.let { runCatching { partJson.fromJson(it.toString()) }.getOrNull() }?.part
            val text = (part as? ModelStreamPart.Part.Text)?.value.orEmpty()
            val thinking = (part as? ModelStreamPart.Part.Thinking)?.value.orEmpty()
            if (text.isEmpty() && thinking.isEmpty()) session
            else session.copy(
                streamingText = session.streamingText + text,
                streamingThinking = session.streamingThinking + thinking,
            )
        }
        "modelStreamEnd" -> {
            val turn = event.turn?.let { runCatching { turnJson.fromJson(it.toString()) }.getOrNull()?.toUi() } ?: run {
                if (session.streamingText.isEmpty() && session.streamingThinking.isEmpty()) return session
                return commitStreamingBuffer(session)
            }
            val normalized = turn.copy(id = turn.id ?: newMessageId(), createdAt = turn.createdAt ?: System.currentTimeMillis())
            // Attaching to a run this device did not start replays the hub
            // buffer from ?since=0, which re-delivers turns already present in
            // the loaded history. Appending them again duplicated the whole
            // reply, and re-adding their tool calls duplicated the run activity.
            // The turn id is the join key: the server assigns one per turn and
            // it survives persistence. The stream buffers still need clearing —
            // they hold a replayed copy of the same text.
            if (session.messages.any { it.id != null && it.id == normalized.id }) {
                return session.copy(streamingText = "", streamingThinking = "")
            }
            var base = session.copy(messages = session.messages + normalized, streamingText = "", streamingThinking = "")
            val toolCalls = turn.content.filterIsInstance<ToolCallPart>()
            if (toolCalls.isEmpty()) return base
            val newEvents = mutableListOf<ActivityEvent>()
            for (part in turn.content) {
                when (part) {
                    is ThinkingPart -> if (part.text.isNotBlank()) newEvents.add(ActivityEvent.Thinking("thinking-${part.text.take(16)}-${System.currentTimeMillis()}", part.text))
                    is TextPart -> if (part.text.isNotBlank()) newEvents.add(ActivityEvent.Text("text-${part.text.take(16)}-${System.currentTimeMillis()}", part.text))
                    is ToolCallPart -> newEvents.add(ActivityEvent.ToolCallEvent(part.call.id, part.call))
                    else -> {}
                }
            }
            base = base.copy(activeToolCalls = toolCalls.map { it.call })
            updateLatestRun(base) { run -> run.copy(events = run.events + newEvents) }
        }
        "toolExecutionStart" -> session.copy(activeToolCalls = event.calls?.mapNotNull(::decodeToolCall) ?: emptyList())
        "toolExecutionResult" -> {
            val r = event.result?.let(::decodeToolResult) ?: return session
            updateLatestRun(session) { run -> run.copy(events = setToolCallResult(run.events, r)) }
        }
        "toolExecutionEnd" -> {
            val results = event.results?.mapNotNull(::decodeToolResult) ?: emptyList()
            var s = session
            for (r in results) {
                s = updateLatestRun(s) { run -> run.copy(events = setToolCallResult(run.events, r)) }
            }
            s.copy(activeToolCalls = emptyList())
        }
        "turnStart" -> {
            // The caller (ChatRepository.sendMessage / attachServerRun) already opened
            // a Working run for this prompt. Opening a second one here would leave an
            // empty run in the list, which shifts every later run's activity onto the
            // wrong user message. Mirrors the hasWorking guard in attachServerRun.
            val hasWorking = session.runs.lastOrNull()?.status == RunStatus.Working
            session.copy(
                running = true,
                runs = if (hasWorking) session.runs else session.runs + RunActivityState(
                    runId = newMessageId(),
                    startedAt = System.currentTimeMillis(),
                    status = RunStatus.Working,
                ),
            )
        }
        "turnEnd" -> {
            // The server emits turnEnd once per provider round-trip — there can be
            // several of these within a single run whenever the agent makes tool
            // calls (turnEnd, then tool execution, then another turnStart/turnEnd).
            // Only sessionEnd marks the run as actually finished; treating turnEnd
            // as terminal here flipped the stop button back to idle after the very
            // first tool call even though the agent kept working.
            commitStreamingBuffer(session)
        }
        "sessionEnd" -> {
            val finalized = updateLatestRun(session) { run ->
                if (run.status != RunStatus.Working) run
                else run.copy(status = RunStatus.Completed, events = finalizePendingToolCalls(run.events),
                    elapsedMs = if (run.startedAt != null) System.currentTimeMillis() - run.startedAt else run.elapsedMs)
            }
            commitStreamingBuffer(finalized.copy(running = false, activeToolCalls = emptyList()))
        }
        "permissionRequest" -> session // structured payload handled at store layer; keep reducer total
        "askQuestion" -> session
        "todoUpdate" -> session.copy(todoItems = event.items?.mapNotNull { runCatching { todoJson.fromJson(it.toString()) }.getOrNull() } ?: emptyList())
        "contextUpdate" -> {
            val snap = event.context?.let { runCatching { contextJson.fromJson(it.toString()) }.getOrNull() }
            if (snap == null) session else session.copy(context = snap)
        }
        "streamReset" -> session.copy(streamingText = "", streamingThinking = "", activeToolCalls = emptyList())
        "error" -> {
            val msg = event.error?.message ?: "Unknown agent error"
            if (msg.lowercase().contains("abort")) {
                val aborted = updateLatestRun(session) { run ->
                    run.copy(status = if (run.status == RunStatus.Working) RunStatus.Aborted else run.status, events = finalizePendingToolCalls(run.events))
                }
                // Server-side abort (a run killed elsewhere) must not strand the
                // partially streamed answer in the buffer.
                commitStreamingBuffer(aborted.copy(running = false, activeToolCalls = emptyList()))
            } else {
                val withRun = updateLatestRun(session) { run ->
                    run.copy(status = if (run.status == RunStatus.Working) RunStatus.Failed else run.status, events = finalizePendingToolCalls(run.events))
                }
                // Commit any partial answer first, then append the error, so the
                // text the user already read isn't lost when the run dies.
                val committed = commitStreamingBuffer(withRun)
                committed.copy(
                    messages = committed.messages + AssistantMessage(id = newMessageId(), content = listOf(TextPart("Error: $msg"))),
                )
            }
        }
        else -> session
    }
}

fun toChatSnapshot(session: ChatSessionState): ChatSnapshot {
    val lastRun = session.runs.lastOrNull()
    val live = lastRun?.events?.filterIsInstance<ActivityEvent.ToolCallEvent>()?.mapNotNull { it.result } ?: emptyList()
    return ChatSnapshot(
        messages = session.messages,
        streamingText = session.streamingText,
        streamingThinking = session.streamingThinking,
        activeToolCalls = session.activeToolCalls,
        liveToolResults = live,
        pendingPermission = session.pendingPermissions.lastOrNull(),
        pendingQuestion = session.pendingQuestions.lastOrNull(),
        pendingPermissions = session.pendingPermissions,
        pendingQuestions = session.pendingQuestions,
        todoItems = session.todoItems,
        subagents = session.subagents,
        running = session.running,
        runs = session.runs,
    )
}
