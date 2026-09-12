package com.console.mobile.data.api

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
import java.net.URLEncoder
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.nullable
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.put

class OkHttpConsoleApi(private val http: HttpTransport) : ConsoleApi {
    private fun enc(v: String): String = URLEncoder.encode(v, "UTF-8")

    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean): List<SessionHeader> {
        val params = mutableMapOf<String, String?>()
        if (cwd != null) params["cwd"] = cwd
        if (projectId != null) params["projectId"] = projectId
        if (onlyDeleted) params["onlyDeleted"] = "true"
        val raw = http.get("/api/sessions", params)
        return http.unwrap(raw, ListSerializer(SessionHeader.serializer()), "list sessions")
    }

    override suspend fun getSession(id: String, limit: Int?, before: Long?): SessionDetailResponse {
        val params = mutableMapOf<String, String?>()
        if (limit != null) params["limit"] = limit.toString()
        if (before != null) params["before"] = before.toString()
        val raw = http.get("/api/sessions/${enc(id)}", params)
        return http.unwrap(raw, SessionDetailResponse.serializer(), "load session")
    }

    override suspend fun createSession(payload: CreateSessionDto): SessionHeader {
        val raw = http.post("/api/sessions", http.encodeBody(CreateSessionDto.serializer(), payload))
        return http.unwrap(raw, SessionHeader.serializer(), "create session")
    }

    override suspend fun updateSession(id: String, payload: UpdateSessionDto): SessionHeader {
        val raw = http.patch("/api/sessions/${enc(id)}", http.encodeBody(UpdateSessionDto.serializer(), payload))
        return http.unwrap(raw, SessionHeader.serializer(), "update session")
    }

    override suspend fun deleteSession(id: String) {
        val raw = http.delete("/api/sessions/${enc(id)}")
        ensureOk(raw, "delete session")
    }

    override suspend fun restoreSession(id: String) {
        val raw = http.post("/api/sessions/${enc(id)}/restore")
        ensureOk(raw, "restore session")
    }

    override suspend fun permanentlyDeleteSession(id: String) {
        val raw = http.delete("/api/sessions/${enc(id)}/permanent")
        ensureOk(raw, "permanently delete session")
    }

    override suspend fun getTodos(id: String): List<TodoItem> {
        val raw = http.get("/api/sessions/${enc(id)}/todos")
        return http.unwrap(raw, ListSerializer(TodoItem.serializer()), "get session todos")
    }

    override suspend fun getSubagents(id: String): List<SubagentInfo> {
        val raw = http.get("/api/sessions/${enc(id)}/subagents")
        return http.unwrap(raw, ListSerializer(SubagentInfo.serializer()), "get session subagents")
    }

    override suspend fun getChanges(id: String): List<SessionFileChange> {
        val raw = http.get("/api/sessions/${enc(id)}/changes")
        return http.unwrap(raw, ListSerializer(SessionFileChange.serializer()), "get session changes")
    }

    override suspend fun abortRun(sessionId: String) {
        val raw = http.post("/api/sessions/${enc(sessionId)}/abort")
        ensureOk(raw, "abort run")
    }

    override suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto) {
        val raw = http.post(
            "/api/sessions/${enc(sessionId)}/answer",
            http.encodeBody(AnswerQuestionDto.serializer(), payload),
        )
        ensureOk(raw, "answer question")
    }

    override suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean {
        val raw = http.post(
            "/api/sessions/${enc(sessionId)}/approve",
            http.encodeBody(ApproveToolPermissionDto.serializer(), payload),
        )
        ensureOk(raw, "approve permission")
        return payload.allow
    }

    override suspend fun getProjects(): List<ProjectInfo> {
        val raw = http.get("/api/projects")
        return http.unwrapOrRaw(raw, ListSerializer(ProjectInfo.serializer()), "list projects")
    }

    override suspend fun addProject(path: String): ProjectInfo {
        val body = buildJsonObject { put("path", path) }.toString()
        val raw = http.post("/api/projects", body)
        return http.unwrapOrRaw(raw, ProjectInfo.serializer(), "add project")
    }

    override suspend fun deleteProject(projectId: String) {
        val raw = http.delete("/api/projects/${enc(projectId)}")
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete project")
    }

    override suspend fun getFsBrowse(path: String?): FsBrowseResult {
        val raw = http.get("/api/fs/browse", mapOf("path" to path))
        return http.unwrapOrRaw(raw, FsBrowseResultSerializer, "browse fs")
    }

    override suspend fun getFsTree(path: String?): List<FsTreeEntry> {
        val raw = http.get("/api/fs/tree", mapOf("path" to path))
        return http.unwrapOrRaw(raw, ListSerializer(FsTreeEntry.serializer()), "load fs tree")
    }

    override suspend fun getFsEntries(path: String, depth: Int): List<FsTreeEntry> {
        val raw = http.get("/api/fs/entries", mapOf("path" to path, "depth" to depth.toString()))
        return http.unwrapOrRaw(raw, ListSerializer(FsTreeEntry.serializer()), "load fs entries")
    }

    override suspend fun searchFiles(root: String, query: String, limit: Int, includeDirs: Boolean): List<FileSearchResult> {
        val raw = http.get(
            "/api/fs/search",
            mapOf("root" to root, "q" to query, "limit" to limit.toString(), "includeDirs" to includeDirs.toString()),
        )
        return try {
            http.unwrapOrRaw(raw, ListSerializer(FileSearchResult.serializer()), "search files")
        } catch (_: Exception) {
            emptyList()
        }
    }

    override suspend fun readFile(path: String): FsFileContent {
        val raw = http.get("/api/fs/file", mapOf("path" to path))
        return http.unwrapOrRaw(raw, FsFileContentSerializer, "read file")
    }

    override suspend fun writeFile(path: String, content: String) {
        val body = buildJsonObject {
            put("path", path)
            put("content", content)
        }.toString()
        val raw = http.post("/api/fs/file", body)
        http.unwrapOrRaw(raw, JsonElement.serializer(), "write file")
    }

    override suspend fun deleteFile(path: String) {
        val raw = http.delete("/api/fs/file", mapOf("path" to path))
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete file")
    }

    override suspend fun createDir(path: String) {
        val body = buildJsonObject { put("path", path) }.toString()
        val raw = http.post("/api/fs/dir", body)
        http.unwrapOrRaw(raw, JsonElement.serializer(), "create dir")
    }

    override suspend fun deleteDir(path: String) {
        val raw = http.delete("/api/fs/dir", mapOf("path" to path))
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete dir")
    }

    override suspend fun getDiff(repoPath: String, filePath: String?): String? {
        val params = mutableMapOf("repoPath" to repoPath)
        if (filePath != null) params["path"] = filePath
        val raw = http.get("/api/git/diff", params)
        return try {
            http.unwrap(raw, GitDiffResponse.serializer().nullable, "load diff")?.diff
        } catch (_: Exception) {
            null
        }
    }

    override suspend fun getGitStatus(path: String): GitStatusSummary? {
        val raw = http.get("/api/git/status", mapOf("path" to path))
        return try {
            http.unwrap(raw, GitStatusSummary.serializer().nullable, "load git status")
        } catch (_: Exception) {
            null
        }
    }

    override suspend fun listBranches(repoPath: String): GitBranchesResponse? {
        val raw = http.get("/api/git/branches", mapOf("path" to repoPath))
        return try {
            http.unwrap(raw, GitBranchesResponse.serializer().nullable, "list git branches")
        } catch (_: Exception) {
            null
        }
    }

    override suspend fun checkoutBranch(repoPath: String, branch: String) {
        val body = buildJsonObject {
            put("path", repoPath)
            put("branch", branch)
        }.toString()
        val raw = http.post("/api/git/checkout", body)
        ensureOk(raw, "checkout branch")
    }

    override suspend fun getProviders(): List<ProviderCatalogEntry> {
        val raw = http.get("/api/providers")
        return http.unwrap(raw, ListSerializer(ProviderCatalogEntry.serializer()), "list providers")
    }

    override suspend fun getProviderModels(providerId: String): List<Model> {
        val raw = http.get("/api/providers/${enc(providerId)}/models")
        return try {
            http.unwrapOrRaw(raw, ProviderModelsResponse.serializer(), "list provider models").models
        } catch (_: Exception) {
            emptyList()
        }
    }

    override suspend fun getApprovalModes(): List<ApprovalModeOption> {
        val raw = http.get("/api/config/approval-modes")
        return http.unwrap(raw, ListSerializer(ApprovalModeOption.serializer()), "list approval modes")
    }

    override suspend fun getAuthStatus(): AuthStatusShim {
        val raw = http.get("/api/auth/status")
        return http.unwrap(raw, AuthStatusShimSerializer, "get auth status")
    }

    override suspend fun getLoginUrl(payload: OAuthLoginUrlDto): LoginUrlResult {
        val raw = http.post("/api/auth/login/url", http.encodeBody(OAuthLoginUrlDto.serializer(), payload))
        return http.unwrap(raw, LoginUrlResultSerializer, "get login url")
    }

    override suspend fun handleCallback(payload: OAuthCallbackDto) {
        val raw = http.post("/api/auth/login/callback", http.encodeBody(OAuthCallbackDto.serializer(), payload))
        http.unwrap(raw, JsonElement.serializer(), "handle auth callback")
    }

    override suspend fun saveProjectId(provider: String, projectId: String?) {
        val body = buildJsonObject {
            put("provider", provider)
            projectId?.let { put("projectId", it) }
        }.toString()
        val raw = http.post("/api/auth/project-id", body)
        http.unwrapOrRaw(raw, JsonElement.serializer(), "save project id")
    }

    override suspend fun listSlashCommands(sessionId: String?): List<SlashCommandInfo> {
        val path = if (sessionId != null) "/api/assist/${enc(sessionId)}/commands" else "/api/assist/commands"
        val raw = http.get(path)
        return http.unwrapOrRaw(raw, ListSerializer(SlashCommandInfo.serializer()), "list slash commands")
    }

    override suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): FileSearchResponse {
        val path = if (sessionId != null) "/api/assist/${enc(sessionId)}/search" else "/api/assist/search"
        val params = mutableMapOf("q" to query)
        if (root != null) params["root"] = root
        val raw = http.get(path, params)
        return http.unwrapOrRaw(raw, FileSearchResponse.serializer(), "assist search")
    }

    override suspend fun getProviderUsage(providerId: String): UsageReport? {
        val raw = http.get("/api/providers/${enc(providerId)}/usage")
        return try {
            http.unwrap(raw, UsageReport.serializer().nullable, "get usage for $providerId")
        } catch (_: Exception) {
            null
        }
    }

    override suspend fun getAllUsage(): Map<String, UsageReport?> {
        val raw = http.get("/api/usage")
        return try {
            http.unwrap(raw, MapSerializer(String.serializer(), UsageReport.serializer().nullable), "get all usage")
        } catch (_: Exception) {
            emptyMap()
        }
    }

    override suspend fun listFavorites(): List<ModelFavorite> {
        val raw = http.get("/api/model-favorites")
        return http.unwrap(raw, ListSerializer(ModelFavorite.serializer()), "list model favorites")
    }

    override suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean) {
        val body = buildJsonObject {
            put("provider", favorite.provider)
            put("modelId", favorite.modelId)
            put("favorite", isFavorite)
        }.toString()
        val raw = http.put("/api/model-favorites", body)
        http.unwrap(raw, JsonElement.serializer(), "update model favorite")
    }

    private fun ensureOk(raw: String, action: String) {
        try {
            val root = ConsoleJson.parseToJsonElement(raw) as? JsonObject
            val success = (root?.get("success") as? JsonPrimitive)?.booleanOrNull
            if (success == false) {
                throw ApiException((root?.get("error") as? JsonPrimitive)?.contentOrNull ?: "Failed to $action")
            }
        } catch (e: ApiException) {
            throw e
        } catch (_: Exception) {
        }
    }

    companion object {
        private val FsBrowseResultSerializer = FsBrowseResult.serializer()
        private val FsFileContentSerializer = FsFileContent.serializer()
        private val LoginUrlResultSerializer = LoginUrlResult.serializer()
        private val AuthStatusShimSerializer = AuthStatusShim.serializer()
    }
}

@kotlinx.serialization.Serializable
private data class ProviderModelsResponse(val provider: String, val models: List<Model> = emptyList())
