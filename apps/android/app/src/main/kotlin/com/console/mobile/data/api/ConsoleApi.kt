package com.console.mobile.data.api

import com.console.mobile.data.model.AnswerQuestionDto
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.ApproveToolPermissionDto
import console.v1.ConsoleSettings
import com.console.mobile.data.model.CreateSessionDto
import console.v1.FileSearchResult
import console.v1.FsBrowseResult
import console.v1.FsDirectoryTree
import console.v1.FsFileContent
import console.v1.FsTreeEntry
import console.v1.GitBranchesResponse
import console.v1.GitDiffResponse
import console.v1.GitStatusSummary
import com.console.mobile.data.model.McpOAuthCallbackPayload
import com.console.mobile.data.model.McpSavePayload
import console.v1.McpServerStatus
import console.v1.Model
import console.v1.ModelFavorite
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.OAuthLoginUrlDto
import console.v1.ProjectInfo
import console.v1.QueuedPrompt
import console.v1.ProviderCatalogEntry
import com.console.mobile.data.model.RunPromptDto
import com.console.mobile.data.model.SessionDetailResponse
import console.v1.SessionFileChange
import console.v1.SessionHeader
import console.v1.SubagentInfo
import console.v1.TodoItem
import console.v1.UsageReport

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
    suspend fun getChangeDiff(id: String, path: String, turnIndex: Int): String?
    suspend fun markChangeReviewed(id: String, path: String, turnIndex: Int, reviewed: Boolean)
    // queue (staged next-turn prompt; Wire decode, hand request bodies)
    suspend fun getQueuedPrompt(id: String): QueuedPrompt?
    suspend fun queuePrompt(id: String, payload: RunPromptDto): QueuedPrompt
    suspend fun editQueuedPrompt(id: String, payload: RunPromptDto): QueuedPrompt
    suspend fun clearQueuedPrompt(id: String): Boolean
    suspend fun steerRun(id: String, payload: RunPromptDto)
    // run
    suspend fun abortRun(sessionId: String)
    suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto)
    suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean
    // fs
    suspend fun getProjects(): List<ProjectInfo>
    suspend fun addProject(path: String): ProjectInfo
    suspend fun deleteProject(projectId: String)
    suspend fun getFsBrowse(path: String?, showHidden: Boolean = false): FsBrowseResult
    suspend fun getFsTree(path: String?): FsDirectoryTree
    suspend fun getFsEntries(path: String, depth: Int = 1): List<FsTreeEntry>
    suspend fun searchFiles(root: String, query: String, limit: Int = 20, includeDirs: Boolean = true): List<FileSearchResult>
    suspend fun readFile(path: String): FsFileContent
    suspend fun writeFile(path: String, content: String)
    suspend fun deleteFile(path: String)
    suspend fun createDir(path: String)
    suspend fun deleteDir(path: String)
    // git
    suspend fun getGitStatus(path: String): GitStatusSummary?
    suspend fun listBranches(repoPath: String): GitBranchesResponse?
    suspend fun checkoutBranch(repoPath: String, branch: String)
    suspend fun getSessionContext(sessionId: String): console.v1.ContextSnapshot?
    /** Turn a message-less session into a worktree session, branching off its cwd. */
    suspend fun attachWorktree(sessionId: String, branch: String?, baseBranch: String?): SessionHeader
    // providers / config
    suspend fun getProviders(): List<ProviderCatalogEntry>
    suspend fun getProviderModels(providerId: String): List<Model>
    suspend fun getApprovalModes(): List<ApprovalModeOption>
    // auth
    suspend fun getAuthStatus(): console.v1.AuthStatusResponse
    suspend fun getLoginUrl(payload: OAuthLoginUrlDto): console.v1.OAuthLoginUrlResponse
    suspend fun handleCallback(payload: OAuthCallbackDto)
    suspend fun saveProjectId(provider: String, projectId: String?)
    // assist
    suspend fun listSlashCommands(sessionId: String?): List<console.v1.SlashCommandInfo>
    suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): console.v1.AssistFileSearchResponse
    // usage
    suspend fun getProviderUsage(providerId: String): UsageReport?
    suspend fun getAllUsage(): Map<String, UsageReport?>
    // mcp
    suspend fun listMcpServers(): List<McpServerStatus>
    suspend fun saveMcpServer(payload: McpSavePayload): McpServerStatus
    suspend fun updateMcpServer(id: String, payload: McpSavePayload): McpServerStatus
    suspend fun deleteMcpServer(id: String)
    suspend fun connectMcpServer(id: String, redirectUri: String?)
    suspend fun disconnectMcpServer(id: String)
    suspend fun forwardMcpOAuthCallback(id: String, payload: McpOAuthCallbackPayload)
    // favorites
    suspend fun listFavorites(): List<ModelFavorite>
    suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean)
    // settings (model roles)
    suspend fun getSettings(): ConsoleSettings
    /** PATCH /api/settings — a null reference clears the role. */
    suspend fun updateModelRoles(roles: Map<String, String?>): ConsoleSettings
    // devices
    suspend fun getDevices(): List<console.v1.DeviceDescriptor>
    suspend fun getDeviceDiagnostics(): console.v1.DeviceDiagnostics
    suspend fun bootDevice(id: String, platform: String)
    suspend fun shutdownDevice(id: String, platform: String)
    suspend fun interactDevice(id: String, platform: String, action: console.v1.DeviceActionRequest)
}

