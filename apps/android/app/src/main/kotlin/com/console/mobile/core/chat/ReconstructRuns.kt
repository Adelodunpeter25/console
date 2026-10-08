package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.ThinkingPart
import com.console.mobile.data.model.ToolCallPart
import com.console.mobile.data.model.ToolResult
import com.console.mobile.data.model.ToolResultMessage
import com.console.mobile.data.model.UserMessage

fun reconstructRuns(messages: List<AgentMessage>): List<RunActivityState> {
    val runs = mutableListOf<RunActivityState>()
    var current: RunActivityState? = null
    var runIndex = 0
    val pending = mutableMapOf<String, ToolResult>()
    for (i in messages.indices) {
        val msg = messages[i]
        if (msg is UserMessage) {
            if (current != null) {
                var done = finalizeRun(current, messages, i - 1)
                done = applyPending(done, pending)
                runs.add(done)
                pending.clear()
            }
            current = RunActivityState(runId = "reconstructed-${runIndex++}", startedAt = msg.createdAt, status = RunStatus.Completed)
        } else if (current != null) {
            var cur: RunActivityState = current
            when (msg) {
                is AssistantMessage -> {
                    if (msg.content.any { it is ToolCallPart }) {
                        for (part in msg.content) {
                            when (part) {
                                is ThinkingPart -> if (part.text.isNotBlank()) cur = cur.copy(events = cur.events + ActivityEvent.Thinking("reconstructed-thinking-$runIndex-${cur.events.size}", part.text))
                                is TextPart -> if (part.text.isNotBlank()) cur = cur.copy(events = cur.events + ActivityEvent.Text("reconstructed-text-$runIndex-${cur.events.size}", part.text))
                                is ToolCallPart -> cur = cur.copy(events = cur.events + ActivityEvent.ToolCallEvent(part.call.id, part.call))
                                else -> {}
                            }
                        }
                        current = cur
                    }
                }
                is ToolResultMessage -> for (r in msg.results) pending[r.toolCallId] = r
            }
        }
    }
    if (current != null) {
        var done = finalizeRun(current, messages, messages.lastIndex)
        done = applyPending(done, pending)
        runs.add(done)
    }
    return runs
}

/**
 * Runs for a session whose latest prompt is still executing server-side.
 *
 * The UI pairs runs with user messages by position, so the live run must be
 * the one belonging to the newest user message. Appending a fresh run after
 * [reconstructRuns] (which already built one for that message) left it
 * orphaned: every streamed tool call landed in a run no message rendered,
 * which is why a run started elsewhere showed only the stop button.
 *
 * The reconstructed run for the latest prompt is reopened as Working, keeping
 * the persisted activity and any live events already collected.
 */
fun withLiveRun(
    messages: List<AgentMessage>,
    runs: List<RunActivityState>,
    nowMs: Long = System.currentTimeMillis(),
): List<RunActivityState> {
    val live = runs.lastOrNull()?.takeIf { it.status == RunStatus.Working }
    val base = reconstructRuns(messages)
    if (base.isEmpty()) {
        return if (live != null) runs else runs + RunActivityState(
            runId = newMessageId(),
            startedAt = nowMs,
            status = RunStatus.Working,
        )
    }
    val last = base.last()
    // Tool calls share their call id between history and the stream; text and
    // thinking events get per-source ids, so they're matched on content.
    val known = last.events.mapTo(HashSet()) { it.id }
    val knownText = last.events.mapNotNullTo(HashSet()) {
        when (it) {
            is ActivityEvent.Text -> it.text
            is ActivityEvent.Thinking -> it.text
            else -> null
        }
    }
    val extra = live?.events?.filterNot {
        it.id in known || (it is ActivityEvent.Text && it.text in knownText) ||
            (it is ActivityEvent.Thinking && it.text in knownText)
    }.orEmpty()
    val liveResults = live?.events.orEmpty()
        .filterIsInstance<ActivityEvent.ToolCallEvent>()
        .mapNotNull { e -> e.result?.let { e.id to it } }
        .toMap()
    val reopened = last.copy(
        runId = live?.runId ?: last.runId,
        startedAt = live?.startedAt ?: last.startedAt ?: nowMs,
        elapsedMs = 0,
        status = RunStatus.Working,
        events = last.events.map { e ->
            if (e is ActivityEvent.ToolCallEvent && e.result == null) e.copy(result = liveResults[e.id]) else e
        } + extra,
    )
    return base.dropLast(1) + reopened
}

/**
 * Merge a freshly fetched history page into a session whose run is live.
 *
 * The server's page wins: it holds the prompt and every finished turn under
 * server ids. Local copies are dropped because the prompt sent from this
 * device carries a client id that never matches, and keeping both rendered
 * the prompt twice and shifted every run onto the wrong message.
 *
 * The only local rows kept are assistant turns that streamed in after the
 * page was served — those follow the last local row the server also has,
 * and their ids are server turn ids, so they can't duplicate anything.
 */
fun mergeLiveHistory(server: List<AgentMessage>, local: List<AgentMessage>): List<AgentMessage> {
    val serverIds = server.mapNotNullTo(HashSet()) { it.id }
    val lastShared = local.indexOfLast { it.id != null && it.id in serverIds }
    if (lastShared < 0) return server
    val tail = local.drop(lastShared + 1).filter { it !is UserMessage && it.id !in serverIds }
    return server + tail
}

private fun applyPending(run: RunActivityState, results: Map<String, ToolResult>): RunActivityState {
    if (results.isEmpty()) return run
    val updated = run.events.map { e ->
        if (e is ActivityEvent.ToolCallEvent && results.containsKey(e.call.id)) e.copy(result = results[e.call.id]) else e
    }
    return run.copy(events = updated)
}

private fun finalizeRun(run: RunActivityState, messages: List<AgentMessage>, lastIndex: Int): RunActivityState {
    val last = messages.getOrNull(lastIndex)
    val started = run.startedAt
    val lastAt = last?.createdAt
    return if (started != null && lastAt != null && lastAt > started) run.copy(elapsedMs = lastAt - started) else run
}
