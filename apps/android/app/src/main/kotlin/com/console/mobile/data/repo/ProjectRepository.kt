package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import console.v1.ProjectInfo
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.ProjectStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Projects: the list, add, clone, delete, and the connected GitHub account's repos. */
class ProjectRepository(
    private val api: ConsoleApi,
    private val projectState: ProjectStateHolder,
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
}
