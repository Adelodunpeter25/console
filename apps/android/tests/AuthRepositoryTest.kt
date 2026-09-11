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
import com.console.mobile.data.model.ProviderAuthStatus
import com.console.mobile.data.model.ProviderCatalogEntry
import com.console.mobile.data.model.SessionDetailResponse
import com.console.mobile.data.model.SessionFileChange
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.AuthStateHolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest

private class AuthApiFake : ConsoleApi {
    var status = AuthStatusShim(
        antigravity = ProviderAuthStatus(loggedIn = true, email = "a@x.com", projectId = "p1", configuredProjectId = "cp1"),
        codex = ProviderAuthStatus(loggedIn = false),
        devin = ProviderAuthStatus(loggedIn = true),
    )
    var lastSavedProvider: String? = null
    var lastSavedProjectId: String? = null
    var lastHandledCallback: OAuthCallbackDto? = null

    override suspend fun getAuthStatus(): AuthStatusShim = status
    override suspend fun getLoginUrl(payload: OAuthLoginUrlDto): LoginUrlResult =
        LoginUrlResult("https://auth.example.com/${payload.provider}", "state1", "http://localhost:8085/cb")
    override suspend fun handleCallback(payload: OAuthCallbackDto) {
        lastHandledCallback = payload
        status = status.copy(codex = ProviderAuthStatus(loggedIn = true))
    }
    override suspend fun saveProjectId(provider: String, projectId: String?) {
        lastSavedProvider = provider
        lastSavedProjectId = projectId
    }

    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean): List<SessionHeader> = emptyList()
    override suspend fun getSession(id: String, limit: Int?, before: Long?): SessionDetailResponse = throw UnsupportedOperationException()
    override suspend fun createSession(payload: CreateSessionDto): SessionHeader = throw UnsupportedOperationException()
    override suspend fun updateSession(id: String, payload: UpdateSessionDto): SessionHeader = throw UnsupportedOperationException()
    override suspend fun deleteSession(id: String) {}
    override suspend fun restoreSession(id: String) {}
    override suspend fun permanentlyDeleteSession(id: String) {}
    override suspend fun getTodos(id: String): List<TodoItem> = emptyList()
    override suspend fun getSubagents(id: String): List<SubagentInfo> = emptyList()
    override suspend fun getChanges(id: String): List<SessionFileChange> = emptyList()
    override suspend fun abortRun(sessionId: String) {}
    override suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto) {}
    override suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean = payload.allow
    override suspend fun getProjects(): List<ProjectInfo> = emptyList()
    override suspend fun addProject(path: String): ProjectInfo = throw UnsupportedOperationException()
    override suspend fun deleteProject(projectId: String) {}
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
    override suspend fun getProviders(): List<ProviderCatalogEntry> = emptyList()
    override suspend fun getProviderModels(providerId: String): List<Model> = emptyList()
    override suspend fun getApprovalModes(): List<ApprovalModeOption> = emptyList()
    override suspend fun listSlashCommands(sessionId: String?): List<SlashCommandInfo> = emptyList()
    override suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): FileSearchResponse =
        FileSearchResponse("", query, emptyList())
    override suspend fun getProviderUsage(providerId: String): UsageReport? = null
    override suspend fun getAllUsage(): Map<String, UsageReport?> = emptyMap()
    override suspend fun listFavorites(): List<ModelFavorite> = emptyList()
    override suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean) {}
}

@OptIn(ExperimentalCoroutinesApi::class)
class AuthRepositoryTest {
    @Test fun loadStatusUpdatesState() = runTest {
        val fake = AuthApiFake()
        val holder = AuthStateHolder()
        val repo = AuthRepository(fake, holder, this)

        repo.loadStatus()
        // launch is dispatched; yield
        testScheduler.advanceUntilIdle()

        val s = holder.state.value
        assertEquals(true, s.status?.get("antigravity")?.loggedIn)
        assertEquals("a@x.com", s.status?.get("antigravity")?.email)
        assertEquals(false, s.status?.get("codex")?.loggedIn)
        assertEquals(true, s.status?.get("devin")?.loggedIn)
        assertEquals("cp1", s.projectIds["antigravity"])
        assertFalse(s.loading)
    }

    @Test fun loginUrlResolution() = runTest {
        val fake = AuthApiFake()
        val holder = AuthStateHolder()
        val repo = AuthRepository(fake, holder, this)

        val res = repo.getLoginUrl("antigravity")
        assertEquals("https://auth.example.com/antigravity", res.authUrl)
    }

    @Test fun submitCallbackRefreshesStatus() = runTest {
        val fake = AuthApiFake()
        val holder = AuthStateHolder()
        val repo = AuthRepository(fake, holder, this)

        repo.submitCallback("codex", "code123", "state1")
        testScheduler.advanceUntilIdle()

        assertEquals("codex", fake.lastHandledCallback?.provider)
        assertEquals("code123", fake.lastHandledCallback?.code)
        assertEquals(true, holder.state.value.status?.get("codex")?.loggedIn)
    }

    @Test fun saveProjectIdPostsAndUpdatesState() = runTest {
        val fake = AuthApiFake()
        val holder = AuthStateHolder()
        val repo = AuthRepository(fake, holder, this)

        repo.saveProjectId("antigravity", "my-cloud-project")
        assertEquals("antigravity", fake.lastSavedProvider)
        assertEquals("my-cloud-project", fake.lastSavedProjectId)
        assertEquals("my-cloud-project", holder.state.value.projectIds["antigravity"])
    }
}
