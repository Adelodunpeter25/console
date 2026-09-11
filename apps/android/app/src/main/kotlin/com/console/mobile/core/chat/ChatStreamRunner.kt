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
    val runs = session.runs.toMutableList()
    if (runs.isNotEmpty()) {
        val latest = runs.last()
        runs[runs.lastIndex] = latest.copy(
            status = if (hadError) RunStatus.Failed else RunStatus.Completed,
            elapsedMs = if (latest.startedAt != null) nowMs - latest.startedAt else latest.elapsedMs,
        )
    }
    return session.copy(running = false, streamingText = "", streamingThinking = "", activeToolCalls = emptyList(), runs = runs)
}

fun abortSessionRun(session: ChatSessionState, nowMs: Long = System.currentTimeMillis()): ChatSessionState {
    val runs = session.runs.toMutableList()
    if (runs.isNotEmpty()) {
        val latest = runs.last()
        if (latest.status == RunStatus.Working) {
            runs[runs.lastIndex] = latest.copy(
                status = RunStatus.Aborted,
                elapsedMs = if (latest.startedAt != null) nowMs - latest.startedAt else latest.elapsedMs,
            )
        }
    }
    return session.copy(running = false, streamingText = "", streamingThinking = "", pendingQuestions = emptyList(), pendingPermissions = emptyList(), activeToolCalls = emptyList(), runs = runs)
}
