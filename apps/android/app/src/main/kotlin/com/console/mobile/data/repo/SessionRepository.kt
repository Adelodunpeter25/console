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

/**
 * Session list/detail repository. Port of useProjectStore session-header
 * refresh + hooks/useHomeSessions list loading + useInfiniteSession paging.
 * Thin wrapper over ConsoleApi with in-memory header cache; chat transcript
 * loading delegates to ChatRepository.loadMessages.
 */
class SessionRepository(
    private val api: ConsoleApi,
    private val sessions: SessionStateHolder,
    private val chats: ChatStateHolder,
    private val chatRepo: ChatRepository,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
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

    fun loadDetail(sessionId: String, limit: Int = 100) {
        scope.launch {
            try {
                val detail: SessionDetailResponse = withContext(Dispatchers.IO) { api.getSession(sessionId, limit, null) }
                applyHeader(sessionId, detail.header)
                chatRepo.loadMessages(sessionId, detail.messages)
            } catch (_: Exception) {
            }
        }
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
