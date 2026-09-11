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
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.FsStateHolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.runTest

private class FsApiFake : ConsoleApi {
    val files = mutableMapOf("src/main.ts" to "console.log('hi');")
    val tree = listOf(FsTreeEntry("src", "src", true), FsTreeEntry("main.ts", "src/main.ts", false))

    override suspend fun getFsBrowse(path: String?): FsBrowseResult =
        FsBrowseResult(path ?: "/", null, tree)

    override suspend fun getFsTree(path: String?): List<FsTreeEntry> = tree

    override suspend fun readFile(path: String): FsFileContent =
        FsFileContent(files[path] ?: throw IllegalArgumentException("File not found"), path)

    override suspend fun writeFile(path: String, content: String) {
        files[path] = content
    }

    override suspend fun deleteFile(path: String) {
        files.remove(path)
    }

    override suspend fun createDir(path: String) {}
    override suspend fun deleteDir(path: String) {}

    override suspend fun searchFiles(root: String, query: String, limit: Int, includeDirs: Boolean): List<FileSearchResult> =
        listOf(FileSearchResult("src/main.ts", "/root/src/main.ts", false, 1.0))

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
    override suspend fun getFsEntries(path: String, depth: Int): List<FsTreeEntry> = emptyList()
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
class FsRepositoryTest {
    @Test fun testFsOperations() = runTest {
        val fake = FsApiFake()
        val holder = FsStateHolder()
        val repo = FsRepository(fake, holder, this)

        val browse = repo.browseDirectory("/root")
        assertEquals("/root", browse.currentPath)
        assertEquals(2, browse.entries.size)

        val tree = repo.getDirectoryTree("/root")
        assertEquals(2, tree.size)
        assertTrue(holder.state.value.treesByPath.containsKey("/root"))

        val content = repo.readFile("src/main.ts")
        assertEquals("console.log('hi');", content.content)
        assertEquals("console.log('hi');", holder.state.value.fileContentsByPath["src/main.ts"])

        repo.writeFile("src/main.ts", "console.log('updated');")
        // Cache invalidated on write
        assertNull(holder.state.value.fileContentsByPath["src/main.ts"])

        val search = repo.searchFiles("/root", "main")
        assertEquals(1, search.size)
        assertEquals("src/main.ts", search[0].relativePath)
    }
}
