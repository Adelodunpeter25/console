package com.console.mobile.data.store

import com.console.mobile.data.model.SessionStatus
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

    fun setStatusesSeed(ids: Map<String, SessionStatus>) {
        val cur = _statuses.value.toMutableMap()
        for ((id, s) in ids) cur.putIfAbsent(id, s)
        _statuses.value = cur
    }

    fun setStatus(id: String, status: SessionStatus) { _statuses.value = _statuses.value + (id to status) }
    fun clearStatus(id: String) { _statuses.value = _statuses.value - id }
    fun clearAll() { _statuses.value = emptyMap(); _views.value = emptyMap() }

    fun getView(id: String): SessionViewState = _views.value[id] ?: EMPTY_SESSION_VIEW
    fun setView(id: String, view: SessionViewState) { _views.value = _views.value + (id to view) }
    fun applyHeader(id: String, modelId: String?, provider: String?, cwd: String?, approvalMode: String?, status: SessionStatus?) {
        setView(id, SessionViewState(modelId, provider, cwd, approvalMode ?: "always-ask"))
        if (status != null) setStatus(id, status)
    }
}
