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
    /** Called once a session is gone for good, so chat state (and its draft) can be dropped. */
    private val onSessionGone: (String) -> Unit = {},
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

    /** Repos of the connected GitHub account, for the pickers' GitHub section. */
    suspend fun listGithubRepos(): List<com.console.mobile.data.model.GithubRepo> = withContext(Dispatchers.IO) {
        api.getGithubRepos()
    }

    /**
     * Clone a GitHub repo on the server (into the server's projects root when
     * [parentDir] is empty) and register it, so the pickers can select it like
     * any other project. A second tap on the same repo fails with the server's
     * "target folder already exists" error — the project registered on the
     * first tap is then selectable from the regular list.
     */
    suspend fun cloneProject(url: String, parentDir: String = "", name: String? = null): ProjectInfo = withContext(Dispatchers.IO) {
        val proj = api.cloneProject(url, parentDir, name)
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

    /**
     * Settle (or un-settle) a chat. The server refuses settling while a run is
     * active or a prompt is queued; the exception carries its message and the
     * local mark is only applied after the call succeeds.
     */
    suspend fun settleSession(id: String, settled: Boolean) = withContext(Dispatchers.IO) {
        if (settled) api.settleSession(id) else api.unsettleSession(id)
        withContext(Dispatchers.Main.immediate) {
            projectState.state.value.sessions.firstOrNull { it.id == id }?.let { header ->
                val patch = if (settled) {
                    // Pinned and settled are mutually exclusive: settling unpins.
                    header.copy(settled_at = System.currentTimeMillis(), pinned_at = null)
                } else {
                    header.copy(settled_at = null)
                }
                projectState.patchSession(id, patch)
            }
        }
    }

    suspend fun deleteSession(id: String) = withContext(Dispatchers.IO) {
        try {
            api.deleteSession(id)
        } catch (e: com.console.mobile.data.api.ApiException) {
            // Already gone server-side (deleted from the desktop): the local row
            // and its draft are all that's left, and those still need clearing.
            if (e.code != "404") throw e
        }
        withContext(Dispatchers.Main.immediate) { forgetSession(id) }
    }

    /** The server deleted a chat (another device, or the settled-chat sweep). Main thread only. */
    fun onSessionDeletedRemotely(id: String) {
        if (projectState.state.value.sessions.any { it.id == id }) forgetSession(id)
    }

    private fun forgetSession(id: String) {
        projectState.removeSession(id)
        sessionState.clearStatus(id)
        onSessionGone(id)
        if (appState.state.value.selectedSessionId == id) {
            appState.setSelectedSessionId(null)
            if (appState.state.value.activeTab == com.console.mobile.data.store.MobileTab.Chat) {
                appState.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
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
            onSessionGone(id)
        }
    }

    /** Fork a session through messageId; the new idle session is prepended like a fresh chat. */
    suspend fun forkSession(id: String, messageId: String): SessionHeader = withContext(Dispatchers.IO) {
        val forked = api.forkSession(id, messageId)
        withContext(Dispatchers.Main.immediate) {
            projectState.prependSession(forked)
            sessionState.setStatus(forked.id, com.console.mobile.data.model.SessionStatus.fromValue(forked.status))
        }
        forked
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
