package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.CreateSessionDto
import console.v1.ProjectInfo
import console.v1.SessionHeader
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.SessionStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ProjectRepository(
    private val api: ConsoleApi,
    private val projectState: ProjectStateHolder,
    private val sessionState: SessionStateHolder,
    private val appState: AppStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    fun loadProjects() {
        scope.launch {
            projectState.setLoading(true)
            projectState.setError(null)
            try {
                val list = withContext(Dispatchers.IO) { api.getProjects() }
                projectState.setProjects(list)
            } catch (e: Exception) {
                // Keep whatever we already had, but say so — an unreachable backend
                // used to look exactly like a server with no projects.
                projectState.setLoading(false)
                projectState.setError(e.message ?: "Failed to load projects")
            }
        }
    }

    suspend fun addProject(path: String): ProjectInfo = withContext(Dispatchers.IO) {
        val proj = api.addProject(path)
        withContext(Dispatchers.Main.immediate) {
            projectState.addProject(proj)
        }
        proj
    }

    suspend fun deleteProject(projectId: String) = withContext(Dispatchers.IO) {
        api.deleteProject(projectId)
        withContext(Dispatchers.Main.immediate) {
            projectState.removeProject(projectId)
            if (appState.state.value.selectedProjectId == projectId) {
                appState.setSelectedProjectId(null)
            }
        }
    }

    fun loadSessions() {
        scope.launch {
            projectState.patchSessionsLoading(true)
            try {
                val list = withContext(Dispatchers.IO) { api.getSessions() }
                projectState.setSessions(list)
                sessionState.setStatusesSeed(list.map { h ->
                    h.id to com.console.mobile.data.model.SessionStatus.fromValue(h.status)
                }.toMap())
            } catch (e: Exception) {
                projectState.patchSessionsLoading(false)
                projectState.setError(e.message ?: "Failed to load chat sessions")
            }
        }
    }

    fun loadDeletedSessions() {
        scope.launch {
            projectState.patchDeletedLoading(true)
            try {
                val list = withContext(Dispatchers.IO) { api.getSessions(onlyDeleted = true) }
                projectState.setDeleted(list)
            } catch (e: Exception) {
                projectState.patchDeletedLoading(false)
                projectState.setError(e.message ?: "Failed to load deleted chats")
            }
        }
    }

    suspend fun createSession(
        cwd: String? = null,
        projectId: String? = null,
        title: String? = "New Chat",
    ): SessionHeader = withContext(Dispatchers.IO) {
        val created = api.createSession(
            CreateSessionDto(
                cwd = cwd,
                projectId = projectId,
                title = title,
            ),
        )
        withContext(Dispatchers.Main.immediate) {
            projectState.prependSession(created)
            sessionState.setStatus(created.id, com.console.mobile.data.model.SessionStatus.fromValue(created.status))
        }
        created
    }

    suspend fun updateSession(id: String, dto: UpdateSessionDto): SessionHeader = withContext(Dispatchers.IO) {
        val updated = api.updateSession(id, dto)
        withContext(Dispatchers.Main.immediate) {
            projectState.patchSession(id, updated)
        }
        updated
    }

    suspend fun deleteSession(id: String) = withContext(Dispatchers.IO) {
        api.deleteSession(id)
        withContext(Dispatchers.Main.immediate) {
            projectState.removeSession(id)
            sessionState.clearStatus(id)
            if (appState.state.value.selectedSessionId == id) {
                appState.setSelectedSessionId(null)
            }
        }
    }

    suspend fun restoreSession(id: String) = withContext(Dispatchers.IO) {
        api.restoreSession(id)
        withContext(Dispatchers.Main.immediate) {
            val restored = projectState.state.value.deletedSessions.firstOrNull { it.id == id }
            projectState.removeDeleted(id)
            if (restored != null) {
                projectState.prependSession(restored)
            }
        }
    }

    suspend fun permanentlyDeleteSession(id: String) = withContext(Dispatchers.IO) {
        api.permanentlyDeleteSession(id)
        withContext(Dispatchers.Main.immediate) {
            projectState.removeDeleted(id)
        }
    }

    fun refreshSessionHeader(sessionId: String) {
        scope.launch {
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, 1, null) }
                projectState.patchSession(sessionId, detail.header)
                sessionState.setStatus(sessionId, SessionStatus.fromValue(detail.header.status))
            } catch (_: Exception) {
            }
        }
    }
}
