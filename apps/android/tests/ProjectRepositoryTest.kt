package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.FsBrowseResult
import com.console.mobile.data.api.FsFileContent
import com.console.mobile.data.api.LoginUrlResult
import com.console.mobile.data.model.AnswerQuestionDto
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.ApproveToolPermissionDto
import com.console.mobile.data.model.AuthStatusShim
import com.console.mobile.data.model.CreateSessionDto
import com.console.mobile.data.model.FileSearchResponse
import com.console.mobile.data.model.FileSearchResult
import com.console.mobile.data.model.FsTreeEntry
import com.console.mobile.data.model.GitBranchesResponse
import com.console.mobile.data.model.GitDiffResponse
import com.console.mobile.data.model.GitStatusSummary
import com.console.mobile.data.model.Model
import com.console.mobile.data.model.ModelFavorite
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.OAuthLoginUrlDto
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.ProviderCatalogEntry
import com.console.mobile.data.model.SessionDetailResponse
import com.console.mobile.data.model.SessionFileChange
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.SessionStateHolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest

private class ProjectApiFake : ConsoleApi {
    val projects = mutableListOf(ProjectInfo("p1", "Project 1", "/path/1", 1, 2))
    val sessions = mutableListOf(SessionHeader("s1", "Session 1", "/path/1", "p1", "m", "codex", 1, 2))
    val deletedSessions = mutableListOf(SessionHeader("sd", "Deleted", "/path/1", "p1", "m", "codex", 1, 2))

    override suspend fun getProjects(): List<ProjectInfo> = projects
    override suspend fun addProject(path: String): ProjectInfo {
        val p = ProjectInfo("p_${projects.size + 1}", "New Project", path, 1, 1)
        projects.add(p)
        return p
    }
    override suspend fun deleteProject(projectId: String) {
        projects.removeAll { it.id == projectId }
    }

    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean): List<SessionHeader> =
        if (onlyDeleted) deletedSessions else sessions

    override suspend fun getSession(id: String, limit: Int?, before: Long?): SessionDetailResponse =
        SessionDetailResponse(header = sessions.first { it.id == id })

    override suspend fun createSession(payload: CreateSessionDto): SessionHeader {
        val s = SessionHeader("s_${sessions.size + 1}", payload.title ?: "New Chat", payload.cwd ?: "", payload.projectId, "m", "c", 1, 1, status = SessionStatus.Idle)
        sessions.add(s)
        return s
    }

    override suspend fun updateSession(id: String, payload: UpdateSessionDto): SessionHeader {
        val idx = sessions.indexOfFirst { it.id == id }
        val updated = sessions[idx].copy(title = payload.title ?: sessions[idx].title)
        sessions[idx] = updated
        return updated
    }

    override suspend fun deleteSession(id: String) {
        val s = sessions.firstOrNull { it.id == id }
        if (s != null) {
            sessions.remove(s)
            deletedSessions.add(s)
        }
    }

    override suspend fun restoreSession(id: String) {
        val s = deletedSessions.firstOrNull { it.id == id }
        if (s != null) {
            deletedSessions.remove(s)
            sessions.add(s)
        }
    }

    override suspend fun permanentlyDeleteSession(id: String) {
        deletedSessions.removeAll { it.id == id }
    }

    override suspend fun getTodos(id: String): List<TodoItem> = emptyList()
    override suspend fun getSubagents(id: String): List<SubagentInfo> = emptyList()
    override suspend fun getChanges(id: String): List<SessionFileChange> = emptyList()
    override suspend fun abortRun(sessionId: String) {}
    override suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto) {}
    override suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean = payload.allow
    override suspend fun getFsBrowse(path: String?): FsBrowseResult = FsBrowseResult("", null, emptyList())
    override suspend fun getFsTree(path: String?): List<FsTreeEntry> = emptyList()
    override suspend fun getFsEntries(path: String, depth: Int): List<FsTreeEntry> = emptyList()
    override suspend fun searchFiles(root: String, query: String, limit: Int, includeDirs: Boolean) = emptyList<FileSearchResult>()
    override suspend fun readFile(path: String): FsFileContent = FsFileContent("", path)
    override suspend fun writeFile(path: String, content: String) {}
    override suspend fun deleteFile(path: String) {}
    override suspend fun createDir(path: String) {}
    override suspend fun deleteDir(path: String) {}
    override suspend fun getDiff(repoPath: String, filePath: String?): String? = null
    override suspend fun getGitStatus(path: String): GitStatusSummary? = null
    override suspend fun listBranches(repoPath: String): GitBranchesResponse? = null
    override suspend fun checkoutBranch(repoPath: String, branch: String) {}
    override suspend fun getProviders(): List<ProviderCatalogEntry> = emptyList()
    override suspend fun getProviderModels(providerId: String): List<Model> = emptyList()
    override suspend fun getApprovalModes(): List<ApprovalModeOption> = emptyList()
    override suspend fun getAuthStatus(): AuthStatusShim = AuthStatusShim()
    override suspend fun getLoginUrl(payload: OAuthLoginUrlDto): LoginUrlResult = LoginUrlResult("", "", "")
    override suspend fun handleCallback(payload: OAuthCallbackDto) {}
    override suspend fun saveProjectId(provider: String, projectId: String?) {}
    override suspend fun listSlashCommands(sessionId: String?): List<SlashCommandInfo> = emptyList()
    override suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): FileSearchResponse = FileSearchResponse("", query, emptyList())
    override suspend fun getProviderUsage(providerId: String): UsageReport? = null
    override suspend fun getAllUsage(): Map<String, UsageReport?> = emptyMap()
    override suspend fun listFavorites(): List<ModelFavorite> = emptyList()
    override suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean) {}
}

@OptIn(ExperimentalCoroutinesApi::class)
class ProjectRepositoryTest {
    @Test fun testProjectsLifecycle() = runTest {
        val fake = ProjectApiFake()
        val projState = ProjectStateHolder()
        val sessionState = SessionStateHolder()
        val appState = AppStateHolder()
        val repo = ProjectRepository(fake, projState, sessionState, appState, this)

        repo.loadProjects()
        advanceUntilIdle()
        assertEquals(1, projState.state.value.projects.size)

        val added = repo.addProject("/path/2")
        assertEquals(2, projState.state.value.projects.size)

        appState.setSelectedProjectId(added.id)
        repo.deleteProject(added.id)
        assertEquals(1, projState.state.value.projects.size)
        assertNull(appState.state.value.selectedProjectId)
    }

    @Test fun testSessionsLifecycle() = runTest {
        val fake = ProjectApiFake()
        val projState = ProjectStateHolder()
        val sessionState = SessionStateHolder()
        val appState = AppStateHolder()
        val repo = ProjectRepository(fake, projState, sessionState, appState, this)

        repo.loadSessions()
        advanceUntilIdle()
        assertEquals(1, projState.state.value.sessions.size)

        val created = repo.createSession(cwd = "/path/1", projectId = "p1", title = "New Session")
        assertEquals(2, projState.state.value.sessions.size)
        assertEquals(SessionStatus.Idle, sessionState.statuses.value[created.id])

        repo.updateSession(created.id, UpdateSessionDto(title = "Renamed"))
        assertEquals("Renamed", projState.state.value.sessions.first { it.id == created.id }.title)

        repo.deleteSession(created.id)
        assertNull(projState.state.value.sessions.firstOrNull { it.id == created.id })

        repo.loadDeletedSessions()
        advanceUntilIdle()
        assertTrue(projState.state.value.deletedSessions.any { it.id == created.id })

        repo.restoreSession(created.id)
        assertTrue(projState.state.value.sessions.any { it.id == created.id })
    }
}
