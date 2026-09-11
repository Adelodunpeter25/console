package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
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
import com.console.mobile.data.model.GitBranchInfo
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
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer

private class GitApiFake : ConsoleApi {
    override suspend fun getGitStatus(path: String): GitStatusSummary? =
        GitStatusSummary(branch = "main", clean = true, files = emptyList())

    override suspend fun getDiff(repoPath: String, filePath: String?): String? = "diff content"
    override suspend fun listBranches(repoPath: String): GitBranchesResponse? =
        GitBranchesResponse(branches = listOf(GitBranchInfo("main", true), GitBranchInfo("dev", false)), isGitRepository = true)
    override suspend fun checkoutBranch(repoPath: String, branch: String) {}

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
class GitRepositoryTest {
    @Test fun testGitOperations() = runTest {
        val fake = GitApiFake()
        val http = OkHttpClient.Builder().build()
        val client = ConsoleApiClient(http, http, baseUrlOverride = "http://localhost:3000")
        val repo = GitRepository(fake, client, http)

        assertEquals("diff content", repo.getDiff("/repo"))
        assertEquals("main", repo.getStatus("/repo")?.branch)
        assertEquals(2, repo.listBranches("/repo")?.branches?.size)

        val projectBranches = repo.fetchBranchesForProjects(listOf(ProjectInfo("p1", "proj", "/repo", 1, 2)))
        assertEquals("main", projectBranches["p1"])
    }

    @Test fun testWatchStatusSseFlow() = runTest {
        val server = MockWebServer()
        server.start()
        try {
            server.enqueue(
                MockResponse()
                    .setResponseCode(200)
                    .addHeader("Content-Type", "text/event-stream")
                    .setBody("event: gitStatus\ndata: {\"branch\":\"feature\",\"clean\":true,\"files\":[]}\n\n"),
            )
            val http = OkHttpClient.Builder().connectTimeout(5, TimeUnit.SECONDS).build()
            val client = ConsoleApiClient(http, http, baseUrlOverride = server.url("/").toString().trimEnd('/'))
            val repo = GitRepository(GitApiFake(), client, http)

            val summary = repo.watchStatus("/path").first()
            assertEquals("feature", summary.branch)
            assertEquals(true, summary.clean)
        } finally {
            server.shutdown()
        }
    }
}
