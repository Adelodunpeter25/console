package com.console.mobile.core.chat

val ABORT_MESSAGES = listOf(
    "This operation was aborted.",
    "Run was aborted.",
    "The operation was aborted.",
    "aborted",
)

fun isAbortError(msg: String): Boolean {
    val lower = msg.lowercase()
    return ABORT_MESSAGES.any { lower.contains(it.lowercase()) }
}

fun finalizeSessionRun(session: ChatSessionState, hadError: Boolean, nowMs: Long = System.currentTimeMillis()): ChatSessionState {
    val turns = session.turns.toMutableList()
    val last = turns.lastOrNull()
    val run = last?.run
    if (last != null && run != null) {
        turns[turns.lastIndex] = last.copy(
            run = run.copy(
                status = if (hadError) RunStatus.Failed else RunStatus.Completed,
                elapsedMs = if (run.startedAt != null) nowMs - run.startedAt else run.elapsedMs,
            )
        )
    }
    return commitStreamingBuffer(
        session.copy(running = false, activeToolCalls = emptyList(), turns = turns),
    )
}

fun abortSessionRun(session: ChatSessionState, nowMs: Long = System.currentTimeMillis()): ChatSessionState {
    val turns = session.turns.toMutableList()
    val last = turns.lastOrNull()
    val run = last?.run
    if (last != null && run != null && run.status == RunStatus.Working) {
        turns[turns.lastIndex] = last.copy(
            run = run.copy(
                status = RunStatus.Aborted,
                elapsedMs = if (run.startedAt != null) nowMs - run.startedAt else run.elapsedMs,
            )
        )
    }
    return commitStreamingBuffer(
        session.copy(
            running = false,
            pendingQuestions = emptyList(),
            activeToolCalls = emptyList(),
            turns = turns,
        ),
    )
}
