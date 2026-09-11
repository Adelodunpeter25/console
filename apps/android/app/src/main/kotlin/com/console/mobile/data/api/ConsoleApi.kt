package com.console.mobile.data.api

import com.console.mobile.data.model.AnswerQuestionDto
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.ApproveToolPermissionDto
import com.console.mobile.data.model.AuthStatusShim
import com.console.mobile.data.model.CreateSessionDto
import com.console.mobile.data.model.FileSearchResponse
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
import com.console.mobile.data.model.RunPromptDto
import com.console.mobile.data.model.SessionDetailResponse
import com.console.mobile.data.model.SessionFileChange
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UsageReport

fun getRunStreamPath(sessionId: String, since: Long? = null): String {
    val base = "/api/sessions/$sessionId/run/stream"
    return if (since != null) "$base?since=$since" else base
}

interface ConsoleApi {
    // sessions
    suspend fun getSessions(cwd: String? = null, projectId: String? = null, onlyDeleted: Boolean = false): List<SessionHeader>
    suspend fun getSession(id: String, limit: Int? = null, before: Long? = null): SessionDetailResponse
    suspend fun createSession(payload: CreateSessionDto): SessionHeader
    suspend fun updateSession(id: String, payload: com.console.mobile.data.model.UpdateSessionDto): SessionHeader
    suspend fun deleteSession(id: String)
    suspend fun restoreSession(id: String)
    suspend fun permanentlyDeleteSession(id: String)
    suspend fun getTodos(id: String): List<TodoItem>
    suspend fun getSubagents(id: String): List<SubagentInfo>
    suspend fun getChanges(id: String): List<SessionFileChange>
    // run
    suspend fun abortRun(sessionId: String)
    suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto)
    suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean
    // fs
    suspend fun getProjects(): List<ProjectInfo>
    suspend fun addProject(path: String): ProjectInfo
    suspend fun deleteProject(projectId: String)
    suspend fun getFsBrowse(path: String?): FsBrowseResult
    suspend fun getFsTree(path: String?): List<FsTreeEntry>
    suspend fun getFsEntries(path: String, depth: Int = 1): List<FsTreeEntry>
    suspend fun searchFiles(root: String, query: String, limit: Int = 20, includeDirs: Boolean = true): List<com.console.mobile.data.model.FileSearchResult>
    suspend fun readFile(path: String): FsFileContent
    suspend fun writeFile(path: String, content: String)
    suspend fun deleteFile(path: String)
    suspend fun createDir(path: String)
    suspend fun deleteDir(path: String)
    // git
    suspend fun getDiff(repoPath: String, filePath: String?): String?
    suspend fun getGitStatus(path: String): GitStatusSummary?
    // providers / config
    suspend fun getProviders(): List<ProviderCatalogEntry>
    suspend fun getProviderModels(providerId: String): List<Model>
    suspend fun getApprovalModes(): List<ApprovalModeOption>
    // auth
    suspend fun getAuthStatus(): AuthStatusShim
    suspend fun getLoginUrl(payload: OAuthLoginUrlDto): LoginUrlResult
    suspend fun handleCallback(payload: OAuthCallbackDto)
    suspend fun saveProjectId(provider: String, projectId: String?)
    // assist
    suspend fun listSlashCommands(sessionId: String?): List<SlashCommandInfo>
    suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): FileSearchResponse
    // usage
    suspend fun getProviderUsage(providerId: String): UsageReport?
    suspend fun getAllUsage(): Map<String, UsageReport?>
    // favorites
    suspend fun listFavorites(): List<ModelFavorite>
    suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean)
}

data class FsBrowseResult(val currentPath: String, val parentPath: String?, val entries: List<FsTreeEntry>)
data class FsFileContent(val content: String, val path: String)
data class LoginUrlResult(val authUrl: String, val state: String, val redirectUri: String)
