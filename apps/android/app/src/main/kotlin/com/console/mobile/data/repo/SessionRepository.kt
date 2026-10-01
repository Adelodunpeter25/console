package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.SessionDetailResponse
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.store.SessionViewState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class SessionRepository(
    private val api: ConsoleApi,
    private val sessions: SessionStateHolder,
    private val chats: ChatStateHolder,
    private val chatRepo: ChatRepository,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    companion object {
        /**
         * Newest-page size on open. Matches the server's default page size
         * (`parsePageParams`) so the first paint covers the same span of
         * history the desktop app sees — a smaller window silently omits older
         * runs, and a run's "Worked for Ns" header only renders when its tool
         * calls are in the loaded messages.
         */
        const val FIRST_PAGE = 50
        /** Older-page size once the user scrolls to the top. */
        const val OLDER_PAGE = 50
    }

    private val _headers = MutableStateFlow<List<SessionHeader>>(emptyList())
    val headers: StateFlow<List<SessionHeader>> = _headers.asStateFlow()

    private val _loading = MutableStateFlow(false)
    val loading: StateFlow<Boolean> = _loading.asStateFlow()

    fun refresh(cwd: String? = null, projectId: String? = null) {
        scope.launch {
            _loading.value = true
            try {
                val list = withContext(Dispatchers.IO) { api.getSessions(cwd, projectId) }
                _headers.value = list
                sessions.setStatusesSeed(list.mapNotNull { h ->
                    h.status?.let { h.id to it }
                }.toMap())
            } catch (_: Exception) {
            } finally {
                _loading.value = false
            }
        }
    }

    /**
     * Sessions whose older-page fetch is in flight. Held in memory rather than
     * in [ChatSessionState] so a fast scroll can't enqueue a second request
     * before the `loadingOlder` flag round-trips through the state flow.
     */
    private val olderInFlight = mutableSetOf<String>()

    /** Newest page for a session. Mobile keeps this small — see [FIRST_PAGE]. */
    fun loadDetail(sessionId: String, limit: Int = FIRST_PAGE) {
        scope.launch {
            try {
                val detail: SessionDetailResponse = withContext(Dispatchers.IO) { api.getSession(sessionId, limit, null) }
                applyHeader(sessionId, detail.header)
                chatRepo.loadMessages(sessionId, detail.messages)
                chatRepo.setPagination(sessionId, detail.hasMore, detail.nextCursor)
            } catch (e: Exception) {
                android.util.Log.w("SessionRepository", "loadDetail($sessionId) failed; chat keeps empty state", e)
            }
        }
    }

    /**
     * Fetch the next older page and prepend it. Mirrors the desktop's
     * `load_older_messages_for_pane`: no-op unless the session reports more
     * messages and nothing is already in flight, and a failure leaves the
     * cursor untouched so the user can retry by scrolling again.
     *
     * Returns whether the request was actually dispatched, so an auto-trigger
     * caller can hold off instead of spinning on a failing backend.
     */
    fun loadOlder(sessionId: String, limit: Int = OLDER_PAGE): Boolean {
        val before = chats.get(sessionId).nextCursor ?: return false
        if (!chats.get(sessionId).hasMoreMessages) return false
        if (!olderInFlight.add(sessionId)) return false
        chatRepo.setLoadingOlder(sessionId, true)
        scope.launch {
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, limit, before) }
                chatRepo.prependMessages(sessionId, detail.messages)
                chatRepo.setPagination(sessionId, detail.hasMore, detail.nextCursor)
            } catch (e: Exception) {
                android.util.Log.w("SessionRepository", "loadOlder($sessionId, before=$before) failed", e)
            } finally {
                olderInFlight.remove(sessionId)
                chatRepo.setLoadingOlder(sessionId, false)
            }
        }
        return true
    }

    fun refreshHeader(sessionId: String) {
        scope.launch {
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, 1, null) }
                applyHeader(sessionId, detail.header)
            } catch (_: Exception) {
            }
        }
    }

    private fun applyHeader(sessionId: String, header: SessionHeader) {
        sessions.setView(
            sessionId,
            SessionViewState(
                sessionModelId = header.modelId,
                sessionProvider = header.provider,
                sessionCwd = header.cwd,
                approvalMode = header.approvalMode ?: ApprovalMode.AlwaysAsk.value,
            ),
        )
        header.status?.let { sessions.setStatus(sessionId, it) }
        _headers.value = _headers.value.map { if (it.id == sessionId) header else it }
    }
}
