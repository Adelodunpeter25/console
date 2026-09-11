package com.console.mobile.data.stream

import com.console.mobile.core.chat.isAbortError
import com.console.mobile.data.model.AgentSessionEvent
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * Owns one session's run stream lifecycle: the initial POST /run pipe plus
 * reconnect-with-resume against GET /run/stream?since=<lastSeq>.
 *
 * Port of apps/mobile/stores/chat/run-stream-controller.ts RunStreamController:
 *  - finalize runs at most once per controller lifetime
 *  - transport failure -> up to 3 resumptions with 1s/2s/4s backoff
 *  - 409 (no active run) or terminal done/aborted frame -> finish cleanly
 *  - cancel() (user stop) kills timers/streams; late callbacks are inert
 */
class RunStreamController(
    val sessionId: String,
    private val deps: Deps,
) {
    interface Deps {
        fun handleEvent(event: AgentSessionEvent)
        fun markError(message: String)
        fun finalize(hadError: Boolean)
        fun baseUrl(): String
        fun authToken(): String?
        fun runBodyJson(): String
        fun scope(): kotlinx.coroutines.CoroutineScope
    }

    companion object {
        const val MAX_RECONNECT_ATTEMPTS = 3
        val RECONNECT_BACKOFF_MS = longArrayOf(1_000, 2_000, 4_000)
    }

    private var collectJob: Job? = null
    private var reconnectJob: Job? = null
    private var finished = false
    private var userCancelled = false
    private var hadError = false
    private var awaitingEndAfterFailure = false
    private var attempts = 0
    private var lastSeq: Long? = null
    private var stream: ChatStreamClient? = null

    val isActive: Boolean get() = !finished && !userCancelled
    val lastSeqValue: Long? get() = lastSeq

    fun attachStream(client: ChatStreamClient) {
        stream = client
    }

    /** Start a fresh agent run (POST /api/sessions/:id/run). */
    fun startRun(client: ChatStreamClient) {
        stream = client
        open { client.startRun(deps.baseUrl(), sessionId, deps.runBodyJson(), deps.authToken()) }
    }

    /** Attach to the server-side active run, replaying events newer than since. */
    fun attach(since: Long? = null, client: ChatStreamClient = stream ?: return) {
        stream = client
        open { client.attach(deps.baseUrl(), sessionId, since, deps.authToken()) }
    }

    /** User-initiated stop. */
    fun cancel() {
        userCancelled = true
        reconnectJob?.cancel()
        reconnectJob = null
        collectJob?.cancel()
        collectJob = null
    }

    fun onEvent(event: AgentSessionEvent, seq: Long?) {
        if (userCancelled || finished) return
        if (seq != null) lastSeq = seq
        if (event.type == "done" || event.type == "aborted") {
            finish(false)
            return
        }
        if (event.type == "error" && !isAbortError(event.error?.message.orEmpty())) {
            hadError = true
        }
        deps.handleEvent(event)
    }

    fun onError(message: String, statusCode: Int?) {
        if (userCancelled || finished) return
        // 409 = run settled server-side while disconnected -> finish gracefully.
        if (statusCode == 409) {
            finish(hadError)
            return
        }
        if (attempts < MAX_RECONNECT_ATTEMPTS) {
            attempts += 1
            awaitingEndAfterFailure = true
            val delayMs = RECONNECT_BACKOFF_MS.getOrElse(attempts - 1) { 4_000 }
            val client = stream ?: return
            reconnectJob?.cancel()
            reconnectJob = deps.scope().launch {
                delay(delayMs)
                if (userCancelled || finished) return@launch
                attach(lastSeq, client)
            }
            return
        }
        hadError = true
        deps.markError(message)
    }

    fun onEnd(aborted: Boolean) {
        if (userCancelled || finished) return
        if (awaitingEndAfterFailure) {
            // Failure already scheduled a reconnect — don't finalize here.
            awaitingEndAfterFailure = false
            return
        }
        finish(hadError)
    }

    private fun open(flows: () -> kotlinx.coroutines.flow.Flow<StreamOutcome>) {
        collectJob?.cancel()
        collectJob = deps.scope().launch {
            try {
                flows().collect { outcome ->
                    when (outcome) {
                        is StreamOutcome.Frame -> onEvent(outcome.frame.event, outcome.frame.seq)
                        is StreamOutcome.End -> when (val e = outcome.end) {
                            is StreamEnd.Aborted -> onEnd(true)
                            is StreamEnd.Completed -> onEnd(false)
                            is StreamEnd.Failed -> {
                                onError(e.message, e.statusCode)
                                // Pair with the terminal end unless a reconnect was scheduled.
                                if (!awaitingEndAfterFailure) onEnd(false)
                            }
                        }
                    }
                }
            } catch (_: kotlinx.coroutines.CancellationException) {
                // cancel() path — stay inert.
            }
        }
    }

    private fun finish(withError: Boolean) {
        if (finished) return
        finished = true
        reconnectJob?.cancel()
        reconnectJob = null
        collectJob?.cancel()
        collectJob = null
        deps.finalize(withError)
    }
}
