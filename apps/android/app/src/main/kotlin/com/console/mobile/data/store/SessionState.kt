package com.console.mobile.data.store

import com.console.mobile.data.model.SessionStatus
import console.v1.SessionFileChange
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class SessionViewState(
    val sessionModelId: String? = null,
    val sessionProvider: String? = null,
    val sessionCwd: String? = null,
    val approvalMode: String = "always-ask",
)

val EMPTY_SESSION_VIEW = SessionViewState()

class SessionStateHolder {
    private val _statuses = MutableStateFlow<Map<String, SessionStatus>>(emptyMap())
    val statuses: StateFlow<Map<String, SessionStatus>> = _statuses.asStateFlow()

    private val _views = MutableStateFlow<Map<String, SessionViewState>>(emptyMap())
    val views: StateFlow<Map<String, SessionViewState>> = _views.asStateFlow()

    private val _sessionChanges = MutableStateFlow<Map<String, List<SessionFileChange>>>(emptyMap())
    val sessionChanges: StateFlow<Map<String, List<SessionFileChange>>> = _sessionChanges.asStateFlow()

    fun setStatusesSeed(ids: Map<String, SessionStatus>) {
        val cur = _statuses.value.toMutableMap()
        for ((id, s) in ids) cur.putIfAbsent(id, s)
        _statuses.value = cur
    }

    fun setStatus(id: String, status: SessionStatus) { _statuses.value = _statuses.value + (id to status) }
    fun clearStatus(id: String) { _statuses.value = _statuses.value - id }
    fun clearAll() { _statuses.value = emptyMap(); _views.value = emptyMap(); _sessionChanges.value = emptyMap() }

    fun setSessionChanges(id: String, changes: List<SessionFileChange>) {
        _sessionChanges.value = _sessionChanges.value + (id to changes)
    }

    fun patchReviewed(id: String, path: String, turnIndex: Int, reviewed: Boolean) {
        val cur = _sessionChanges.value.toMutableMap()
        cur[id] = (cur[id] ?: emptyList()).map { c ->
            if (c.path == path && c.turn_index == turnIndex) c.copy(reviewed = reviewed) else c
        }
        _sessionChanges.value = cur
    }

    fun getView(id: String): SessionViewState = _views.value[id] ?: EMPTY_SESSION_VIEW
    fun setView(id: String, view: SessionViewState) { _views.value = _views.value + (id to view) }
    fun applyHeader(id: String, modelId: String?, provider: String?, cwd: String?, approvalMode: String?, status: SessionStatus?) {
        setView(id, SessionViewState(modelId, provider, cwd, approvalMode ?: "always-ask"))
        if (status != null) setStatus(id, status)
    }
}
