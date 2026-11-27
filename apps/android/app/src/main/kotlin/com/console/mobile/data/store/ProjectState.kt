package com.console.mobile.data.store

import console.v1.ProjectInfo
import console.v1.SessionHeader
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

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

    fun setLoading(loading: Boolean) { _state.update { it.copy(loading = loading) } }
    fun setError(error: String?) { _state.update { it.copy(error = error) } }
    fun patchSessionsLoading(loading: Boolean) { _state.update { it.copy(sessionsLoading = loading) } }
    fun patchDeletedLoading(loading: Boolean) { _state.update { it.copy(deletedLoading = loading) } }

    fun setProjects(v: List<ProjectInfo>) {
        _state.update { it.copy(projects = v, loading = false, error = null) }
    }
    fun addProject(p: ProjectInfo) { _state.update { it.copy(projects = it.projects + p) } }
    fun removeProject(id: String) { _state.update { it.copy(projects = it.projects.filter { p -> p.id != id }) } }
    fun setSessions(v: List<SessionHeader>) {
        _state.update { it.copy(sessions = v, sessionsLoading = false, error = null) }
    }
    fun prependSession(s: SessionHeader) { _state.update { it.copy(sessions = listOf(s) + it.sessions) } }
    fun mergeSessions(incoming: List<SessionHeader>) {
        if (incoming.isEmpty()) return
        _state.update { current ->
            val existingIds = current.sessions.mapTo(HashSet()) { it.id }
            val fresh = incoming.filterNot { it.id in existingIds }
            if (fresh.isEmpty()) current else current.copy(sessions = current.sessions + fresh)
        }
    }
    fun patchSession(id: String, patch: SessionHeader) {
        _state.update { it.copy(sessions = it.sessions.map { s -> if (s.id == id) patch else s }) }
    }
    fun unsettleSession(id: String) {
        _state.update {
            it.copy(
                sessions = it.sessions.map { s ->
                    if (s.id == id && s.settled_at != null) s.copy(settled_at = null) else s
                }
            )
        }
    }
    fun updateSessionTitle(id: String, title: String) {
        _state.update {
            it.copy(
                sessions = it.sessions.map { s ->
                    if (s.id == id) s.copy(title = title) else s
                }
            )
        }
    }
    fun removeSession(id: String) { _state.update { it.copy(sessions = it.sessions.filter { s -> s.id != id }) } }
    fun setDeleted(v: List<SessionHeader>) {
        _state.update { it.copy(deletedSessions = v, deletedLoading = false, error = null) }
    }
    fun removeDeleted(id: String) { _state.update { it.copy(deletedSessions = it.deletedSessions.filter { s -> s.id != id }) } }
    fun clear() { _state.update { ProjectState() } }
}
