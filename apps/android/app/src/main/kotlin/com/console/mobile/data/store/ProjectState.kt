package com.console.mobile.data.store

import console.v1.ProjectInfo
import com.console.mobile.data.model.SessionHeader
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class ProjectState(
    val projects: List<ProjectInfo> = emptyList(),
    val loading: Boolean = false,
    val sessions: List<SessionHeader> = emptyList(),
    val sessionsLoading: Boolean = false,
    val deletedSessions: List<SessionHeader> = emptyList(),
    val deletedLoading: Boolean = false,
    // Set when a load fails, so screens can tell "backend unreachable" apart from
    // "genuinely empty". Without this both rendered as the same empty state.
    val error: String? = null,
)

class ProjectStateHolder(initial: ProjectState = ProjectState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<ProjectState> = _state.asStateFlow()

    fun setLoading(loading: Boolean) { _state.value = _state.value.copy(loading = loading) }
    fun setError(error: String?) { _state.value = _state.value.copy(error = error) }
    fun patchSessionsLoading(loading: Boolean) { _state.value = _state.value.copy(sessionsLoading = loading) }
    fun patchDeletedLoading(loading: Boolean) { _state.value = _state.value.copy(deletedLoading = loading) }

    fun setProjects(v: List<ProjectInfo>) {
        _state.value = _state.value.copy(projects = v, loading = false, error = null)
    }
    fun addProject(p: ProjectInfo) { _state.value = _state.value.copy(projects = _state.value.projects + p) }
    fun removeProject(id: String) { _state.value = _state.value.copy(projects = _state.value.projects.filter { it.id != id }) }
    fun setSessions(v: List<SessionHeader>) {
        _state.value = _state.value.copy(sessions = v, sessionsLoading = false, error = null)
    }
    fun prependSession(s: SessionHeader) { _state.value = _state.value.copy(sessions = listOf(s) + _state.value.sessions) }
    fun patchSession(id: String, patch: SessionHeader) {
        _state.value = _state.value.copy(sessions = _state.value.sessions.map { if (it.id == id) patch else it })
    }
    fun removeSession(id: String) { _state.value = _state.value.copy(sessions = _state.value.sessions.filter { it.id != id }) }
    fun setDeleted(v: List<SessionHeader>) {
        _state.value = _state.value.copy(deletedSessions = v, deletedLoading = false, error = null)
    }
    fun removeDeleted(id: String) { _state.value = _state.value.copy(deletedSessions = _state.value.deletedSessions.filter { it.id != id }) }
    fun clear() { _state.value = ProjectState() }
}
