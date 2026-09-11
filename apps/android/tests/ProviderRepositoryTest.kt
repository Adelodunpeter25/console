package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.FsBrowseResult
import com.console.mobile.data.api.FsFileContent
import com.console.mobile.data.api.LoginUrlResult
import com.console.mobile.data.model.AnswerQuestionDto
import com.console.mobile.data.model.ApprovalMode
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
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.ProviderStateHolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest

private class ProviderApiFake : ConsoleApi {
    val providers = listOf(
        ProviderCatalogEntry(
            name = "antigravity",
            displayName = "Antigravity",
            description = "Google",
            models = listOf(Model("gemini-1.5", "antigravity", 1000000, supportsImages = true)),
            authMethod = "oauth",
        ),
        ProviderCatalogEntry(
            name = "codex",
            displayName = "Codex",
            description = "OpenAI",
            models = listOf(Model("o3-mini", "codex", 128000, supportsImages = false)),
            authMethod = "oauth",
        ),
    )

    override suspend fun getProviders(): List<ProviderCatalogEntry> = providers
    override suspend fun getProviderModels(providerId: String): List<Model> =
        providers.firstOrNull { it.name == providerId }?.models ?: emptyList()
    override suspend fun getApprovalModes(): List<ApprovalModeOption> = listOf(
        ApprovalModeOption(ApprovalMode.AlwaysAsk, "Always Ask", "Prompt on every write"),
    )

    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean) = emptyList<SessionHeader>()
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
    override suspend fun listBranches(repoPath: String): GitBranchesResponse? = null
    override suspend fun checkoutBranch(repoPath: String, branch: String) {}
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
class ProviderRepositoryTest {
    @Test fun testProvidersAndModels() = runTest {
        val fake = ProviderApiFake()
        val holder = ProviderStateHolder()
        val repo = ProviderRepository(fake, holder, this)

        repo.loadProviders()
        advanceUntilIdle()
        assertEquals(2, holder.state.value.providers.size)

        val models = repo.loadModels("antigravity")
        assertEquals(1, models.size)
        assertEquals("gemini-1.5", models[0].id)

        // image support check
        assertTrue(repo.supportsImages("antigravity", "gemini-1.5"))
        assertFalse(repo.supportsImages("codex", "o3-mini"))

        // provider resolve
        assertEquals("antigravity", repo.resolveProvider("gemini-1.5", null))
        assertEquals("codex", repo.resolveProvider("o3-mini", null))
    }
}
