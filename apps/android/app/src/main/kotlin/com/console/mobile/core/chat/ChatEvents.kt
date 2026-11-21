package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.BackgroundWork
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.UserMessage
import com.console.mobile.data.model.toUi
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentAssistantMessage as WireAssistantMessage
import console.v1.ContextSnapshot
import console.v1.ModelStreamPart
import console.v1.SubagentActivityItem
import console.v1.SubagentInfo
import console.v1.TodoItem
import console.v1.ToolCall as WireToolCall
import console.v1.ToolResult as WireToolResult
import okio.ByteString.Companion.encodeUtf8
import java.util.UUID

private val todoJson = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(TodoItem::class.java)
private val partJson = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(ModelStreamPart::class.java)
private val wireMoshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
private val toolCallJson = wireMoshi.adapter(WireToolCall::class.java)
private val toolResultJson = wireMoshi.adapter(WireToolResult::class.java)
private val contextJson = wireMoshi.adapter(ContextSnapshot::class.java)
private val turnJson = wireMoshi.adapter(WireAssistantMessage::class.java)
private val queuedPromptJson = wireMoshi.adapter(console.v1.QueuedPrompt::class.java)

private fun decodeToolCall(raw: kotlinx.serialization.json.JsonElement): com.console.mobile.data.model.ToolCall? =
    runCatching { toolCallJson.fromJson(raw.toString()) }.getOrNull()?.toUi()

private fun decodeToolResult(raw: kotlinx.serialization.json.JsonElement): com.console.mobile.data.model.ToolResult? =
    runCatching { toolResultJson.fromJson(raw.toString()) }.getOrNull()?.toUi()

fun newMessageId(): String = try {
    UUID.randomUUID().toString()
} catch (_: Exception) {
    "msg-${System.currentTimeMillis()}-${(Math.random() * 1e6).toInt()}"
}

private fun updateTailTurn(
    session: ChatSessionState,
    update: (TranscriptTurn) -> TranscriptTurn,
): ChatSessionState {
    val turns = session.turns.toMutableList()
    if (turns.isEmpty()) {
        turns.add(update(TranscriptTurn(id = newMessageId(), createdAt = System.currentTimeMillis())))
    } else {
        turns[turns.lastIndex] = update(turns.last())
    }
    return session.copy(turns = turns)
}

private fun updateLatestRun(
    session: ChatSessionState,
    update: (RunActivityState) -> RunActivityState,
): ChatSessionState {
    val last = session.turns.lastOrNull() ?: return session
    val run = last.run ?: return session
    val turns = session.turns.toMutableList()
    turns[turns.lastIndex] = last.copy(run = update(run))
    return session.copy(turns = turns)
}

private fun ensureTailRun(session: ChatSessionState, now: Long = System.currentTimeMillis()): ChatSessionState {
    return updateTailTurn(session) { turn ->
        if (turn.run != null) turn
        else turn.copy(
            run = RunActivityState(
                runId = turn.id.ifEmpty { newMessageId() },
                startedAt = turn.createdAt ?: now,
                status = RunStatus.Working,
                dividerLabel = turn.wakeLabel,
            )
        )
    }
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

private fun upsertBackgroundWork(current: List<BackgroundWork>, work: BackgroundWork): List<BackgroundWork> {
    val index = current.indexOfFirst { it.id == work.id }
    return if (index >= 0) current.toMutableList().also { it[index] = work } else current + work
}

fun commitStreamingBuffer(session: ChatSessionState): ChatSessionState {
    if (session.streamingText.isEmpty() && session.streamingThinking.isEmpty()) {
        return session.copy(streamingText = "", streamingThinking = "")
    }
    val parts = buildList {
        if (session.streamingThinking.isNotEmpty()) add(ThinkingPart(session.streamingThinking))
        if (session.streamingText.isNotEmpty()) add(TextPart(session.streamingText))
    }
    val msg = AssistantMessage(
        id = newMessageId(),
        createdAt = System.currentTimeMillis(),
        content = parts,
    )
    return updateTailTurn(session) { turn ->
        turn.copy(responses = turn.responses + msg)
    }.copy(streamingText = "", streamingThinking = "")
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
            val turnMsg = event.turn?.let { runCatching { turnJson.fromJson(it.toString()) }.getOrNull()?.toUi() } ?: run {
                if (session.streamingText.isEmpty() && session.streamingThinking.isEmpty()) return session
                return commitStreamingBuffer(session)
            }
            val normalized = turnMsg.copy(
                id = turnMsg.id ?: newMessageId(),
                createdAt = turnMsg.createdAt ?: System.currentTimeMillis(),
            )
            if (session.turns.any { t -> t.responses.any { it.id != null && it.id == normalized.id } }) {
                return session.copy(streamingText = "", streamingThinking = "")
            }
            val toolCalls = turnMsg.content.filterIsInstance<ToolCallPart>()
            if (toolCalls.isEmpty()) {
                return updateTailTurn(session) { t ->
                    t.copy(responses = t.responses + normalized)
                }.copy(streamingText = "", streamingThinking = "")
            }
            val withRun = ensureTailRun(session.copy(streamingText = "", streamingThinking = ""))
            val existingCallIds = withRun.turns.lastOrNull()?.run?.events
                ?.filterIsInstance<ActivityEvent.ToolCallEvent>()
                ?.mapTo(HashSet()) { it.call.id }
                .orEmpty()
            if (toolCalls.all { it.call.id in existingCallIds }) {
                return withRun
            }
            val newEvents = mutableListOf<ActivityEvent>()
            for (part in turnMsg.content) {
                when (part) {
                    is ThinkingPart -> if (part.text.isNotBlank()) {
                        newEvents.add(ActivityEvent.Thinking("thinking-${part.text.take(16)}-${System.currentTimeMillis()}", part.text))
                    }
                    is TextPart -> if (part.text.isNotBlank()) {
                        newEvents.add(ActivityEvent.Text("text-${part.text.take(16)}-${System.currentTimeMillis()}", part.text))
                    }
                    is ToolCallPart -> if (part.call.id !in existingCallIds) {
                        newEvents.add(ActivityEvent.ToolCallEvent(part.call.id, part.call))
                    }
                    else -> {}
                }
            }
            updateLatestRun(withRun.copy(activeToolCalls = toolCalls.map { it.call })) { run ->
                run.copy(events = run.events + newEvents)
            }
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
            val prompt = event.prompt
            val lastUser = session.turns.lastOrNull { it.userMessage != null }?.userMessage
            if (!prompt.isNullOrBlank() && (lastUser?.content != prompt || lastUser.source != event.source)) {
                val now = System.currentTimeMillis()
                val settled = commitStreamingBuffer(session)
                val closed = updateLatestRun(settled) { run ->
                    if (run.status != RunStatus.Working) run
                    else run.copy(
                        status = RunStatus.Completed,
                        events = finalizePendingToolCalls(run.events),
                        elapsedMs = if (run.startedAt != null) now - run.startedAt else run.elapsedMs,
                    )
                }
                val newTurn = TranscriptTurn(
                    id = newMessageId(),
                    createdAt = now,
                    userMessage = UserMessage(id = newMessageId(), createdAt = now, content = prompt, source = event.source),
                    run = RunActivityState(runId = newMessageId(), startedAt = now, status = RunStatus.Working),
                )
                return closed.copy(
                    turns = closed.turns + newTurn,
                    running = true,
                    activeToolCalls = emptyList(),
                )
            }
            ensureTailRun(session.copy(running = true))
        }
        "turnEnd" -> commitStreamingBuffer(session)
        "sessionEnd" -> {
            val finalized = updateLatestRun(session) { run ->
                if (run.status != RunStatus.Working) run
                else run.copy(
                    status = RunStatus.Completed,
                    events = finalizePendingToolCalls(run.events),
                    elapsedMs = if (run.startedAt != null) System.currentTimeMillis() - run.startedAt else run.elapsedMs,
                )
            }
            commitStreamingBuffer(finalized.copy(running = false, activeToolCalls = emptyList()))
        }
        "askQuestion" -> session
        "todoUpdate" -> session.copy(todoItems = event.items?.mapNotNull { runCatching { todoJson.fromJson(it.toString()) }.getOrNull() } ?: emptyList())
        "contextUpdate" -> {
            val snap = event.context?.let { runCatching { contextJson.fromJson(it.toString()) }.getOrNull() }
            if (snap == null) session else session.copy(context = snap)
        }
        "queueUpdated" -> {
            val queued = (event.queuedPrompts?.firstOrNull() ?: event.queuedPrompt)?.let { runCatching { queuedPromptJson.fromJson(it.toString()) }.getOrNull() }
            session.copy(queuedPrompt = queued)
        }
        "subagentStart" -> {
            val sid = event.subagentId ?: return session
            val existing = session.subagents.firstOrNull { it.subagent_id == sid }
            val updatedList = if (existing != null) {
                session.subagents.map { s ->
                    if (s.subagent_id == sid) {
                        s.copy(
                            name = event.name ?: s.name,
                            role = event.role ?: s.role,
                            prompt = event.prompt ?: s.prompt,
                            status = "running",
                        )
                    } else s
                }
            } else {
                session.subagents + SubagentInfo(
                    subagent_id = sid,
                    parent_tool_call_id = event.parentToolCallId ?: "",
                    name = event.name ?: "",
                    role = event.role ?: "",
                    prompt = event.prompt ?: "",
                    current_turn = 1,
                    status = "running",
                    activities = emptyList(),
                    created_at = System.currentTimeMillis(),
                    updated_at = System.currentTimeMillis(),
                )
            }
            session.copy(subagents = updatedList)
        }
        "subagentActivity" -> {
            val sid = event.subagentId ?: return session
            val turn = event.turnIndex ?: 1
            val callId = event.toolCallId ?: ""
            val toolName = event.toolName ?: ""
            val status = event.status ?: "running"
            val rawArgs = event.args?.toString() ?: ""
            val argsBytes = rawArgs.encodeUtf8()

            val updatedList = session.subagents.map { s ->
                if (s.subagent_id == sid) {
                    val acts = s.activities.toMutableList()
                    val existingIdx = acts.indexOfFirst { it.tool_call_id == callId }
                    if (existingIdx >= 0) {
                        val currentAct = acts[existingIdx]
                        acts[existingIdx] = currentAct.copy(
                            status = status,
                            error = event.error?.message ?: currentAct.error,
                        )
                    } else {
                        acts.add(
                            SubagentActivityItem(
                                turn_index = turn,
                                tool_call_id = callId,
                                tool_name = toolName,
                                status = status,
                                args = argsBytes,
                                error = event.error?.message,
                            )
                        )
                    }
                    s.copy(
                        current_turn = maxOf(s.current_turn, turn),
                        activities = acts,
                        updated_at = System.currentTimeMillis(),
                    )
                } else s
            }
            session.copy(subagents = updatedList)
        }
        "subagentEnd" -> {
            val sid = event.subagentId ?: return session
            val status = event.status ?: "completed"
            val total = event.totalTurns
            val summary = event.summary
            val updatedList = session.subagents.map { s ->
                if (s.subagent_id == sid) {
                    s.copy(
                        status = status,
                        summary = summary ?: s.summary,
                        current_turn = total ?: s.current_turn,
                        updated_at = System.currentTimeMillis(),
                    )
                } else s
            }
            session.copy(subagents = updatedList)
        }
        "backgroundWorkContinue" -> {
            val items = event.works
            var updated = session.backgroundWork
            for (w in items) {
                updated = upsertBackgroundWork(updated, w)
            }
            val now = System.currentTimeMillis()
            val settled = updateLatestRun(commitStreamingBuffer(session)) { run ->
                if (run.status != RunStatus.Working) run
                else run.copy(
                    status = RunStatus.Completed,
                    events = finalizePendingToolCalls(run.events),
                    elapsedMs = if (run.startedAt != null) now - run.startedAt else run.elapsedMs,
                )
            }
            val label = event.message ?: "Background work completed, continuing agent"
            val wakeTurn = TranscriptTurn(
                id = newMessageId(),
                createdAt = now,
                wakeLabel = label,
                run = RunActivityState(
                    runId = newMessageId(),
                    startedAt = now,
                    status = RunStatus.Working,
                    dividerLabel = label,
                ),
            )
            settled.copy(
                backgroundWork = updated,
                turns = settled.turns + wakeTurn,
            )
        }
        "backgroundWorkStarted", "backgroundWorkProgress", "backgroundWorkCompleted", "backgroundWorkFailed", "backgroundWorkCancelled" -> {
            val items = event.works
            if (items.isEmpty()) return session
            var updated = session.backgroundWork
            for (w in items) {
                updated = upsertBackgroundWork(updated, w)
            }
            session.copy(backgroundWork = updated)
        }
        "streamReset" -> session.copy(streamingText = "", streamingThinking = "", activeToolCalls = emptyList())
        "error" -> {
            val msg = event.error?.message ?: "Unknown agent error"
            if (msg.lowercase().contains("abort")) {
                val aborted = updateLatestRun(session) { run ->
                    run.copy(status = if (run.status == RunStatus.Working) RunStatus.Aborted else run.status, events = finalizePendingToolCalls(run.events))
                }
                commitStreamingBuffer(aborted.copy(running = false, activeToolCalls = emptyList()))
            } else {
                val withRun = updateLatestRun(session) { run ->
                    run.copy(status = if (run.status == RunStatus.Working) RunStatus.Failed else run.status, events = finalizePendingToolCalls(run.events))
                }
                val committed = commitStreamingBuffer(withRun)
                updateTailTurn(committed) { turn ->
                    turn.copy(responses = turn.responses + AssistantMessage(id = newMessageId(), content = listOf(TextPart("Error: $msg"))))
                }
            }
        }
        else -> session
    }
}

fun toChatSnapshot(session: ChatSessionState): ChatSnapshot {
    val lastRun = session.turns.lastOrNull()?.run
    val live = lastRun?.events?.filterIsInstance<ActivityEvent.ToolCallEvent>()?.mapNotNull { it.result } ?: emptyList()
    return ChatSnapshot(
        turns = session.turns,
        streamingText = session.streamingText,
        streamingThinking = session.streamingThinking,
        activeToolCalls = session.activeToolCalls,
        liveToolResults = live,
        pendingQuestion = session.pendingQuestions.lastOrNull(),
        pendingQuestions = session.pendingQuestions,
        todoItems = session.todoItems,
        subagents = session.subagents,
        backgroundWork = session.backgroundWork,
        running = session.running,
        queuedPrompt = session.queuedPrompt,
    )
}
