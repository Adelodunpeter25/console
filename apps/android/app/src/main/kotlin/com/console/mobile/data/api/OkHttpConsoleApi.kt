package com.console.mobile.data.api

import com.console.mobile.data.model.AnswerQuestionDto
import com.console.mobile.data.model.toUi
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.ApproveToolPermissionDto
import com.console.mobile.data.model.BackgroundWork
import com.console.mobile.data.model.BackgroundWorkOutput
import console.v1.AuthStatusResponse
import console.v1.OAuthLoginUrlResponse
import com.console.mobile.data.model.CreateSessionDto
import console.v1.DeviceActionRequest
import console.v1.DeviceDescriptor
import console.v1.DeviceDiagnostics
import console.v1.CreateDirRequest
import console.v1.AssistFileSearchResponse
import console.v1.FileSearchResult
import console.v1.SlashCommandInfo
import console.v1.FsBrowseResult
import console.v1.FsDirectoryTree
import console.v1.FsFileContent
import console.v1.FsTreeEntry
import console.v1.ForwardedPort
import console.v1.WriteFileRequest
import com.console.mobile.data.model.McpOAuthCallbackPayload
import com.console.mobile.data.model.McpSavePayload
import console.v1.McpServerStatus
import console.v1.Model
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ConsoleSettings
import console.v1.ModelFavorite
import console.v1.GitBranchesResponse
import console.v1.GitCheckoutRequest
import console.v1.GitDiffResponse
import console.v1.GitStatusSummary
import console.v1.ProjectInfo
import console.v1.QueuedPrompt
import console.v1.SetFavoriteRequest
import console.v1.UsageReport
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.GitHubTokenDto
import com.console.mobile.data.model.ProviderCredentialRequest
import com.console.mobile.data.model.OAuthLoginUrlDto
import com.console.mobile.data.model.DeviceInfo
import com.console.mobile.data.model.DeviceListData
import com.console.mobile.data.model.PairingData
import com.console.mobile.data.model.PairingGrantData
import com.console.mobile.data.model.PairingGrantRequest
import com.console.mobile.data.model.PairingRequest
import console.v1.ProviderCredentialStatus
import console.v1.ProviderCredentialStatuses
import console.v1.SetProviderCredentialResponse
import console.v1.ProviderCatalogEntry
import console.v1.ProviderModelsResponse
import com.console.mobile.data.model.RunPromptDto
import com.console.mobile.data.model.SessionDetailResponse
import console.v1.SessionFileChange
import console.v1.SessionHeader
import console.v1.SubagentInfo
import console.v1.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import java.net.URLEncoder
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.builtins.MapSerializer
import kotlinx.serialization.builtins.nullable
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject

class OkHttpConsoleApi(private val http: HttpTransport) : ConsoleApi {
    // Moshi JSON layer for Wire types (shared protobuf schema). kotlinx stays
    // for the {success, data} envelope and all non-wire types.
    private val wireMoshi: Moshi by lazy { Moshi.Builder().add(WireJsonAdapterFactory()).build() }
    private val favoriteAdapter by lazy { wireMoshi.adapter(ModelFavorite::class.java) }
    private val setFavoriteAdapter by lazy { wireMoshi.adapter(SetFavoriteRequest::class.java) }
    private val settingsAdapter by lazy { wireMoshi.adapter(ConsoleSettings::class.java) }
    private val projectAdapter by lazy { wireMoshi.adapter(ProjectInfo::class.java) }
    private val usageReportAdapter by lazy { wireMoshi.adapter(UsageReport::class.java) }
    private val sessionAdapter by lazy { wireMoshi.adapter(SessionHeader::class.java) }
    private val providerCatalogAdapter by lazy { wireMoshi.adapter(ProviderCatalogEntry::class.java) }
    private val providerModelsAdapter by lazy { wireMoshi.adapter(ProviderModelsResponse::class.java) }
    private val sessionChangeAdapter by lazy { wireMoshi.adapter(SessionFileChange::class.java) }
    private val queuedPromptAdapter by lazy { wireMoshi.adapter(QueuedPrompt::class.java) }
    private val subagentAdapter by lazy { wireMoshi.adapter(SubagentInfo::class.java) }
    private val messageMoshi by lazy { wireMoshi.adapter(console.v1.AgentMessage::class.java) }
    private val todoAdapter by lazy { wireMoshi.adapter(TodoItem::class.java) }
    private val gitDiffAdapter by lazy { wireMoshi.adapter(GitDiffResponse::class.java) }
    private val gitStatusAdapter by lazy { wireMoshi.adapter(GitStatusSummary::class.java) }
    private val gitBranchesAdapter by lazy { wireMoshi.adapter(GitBranchesResponse::class.java) }
    private val contextAdapter by lazy { wireMoshi.adapter(console.v1.ContextSnapshot::class.java) }
    private val gitCheckoutAdapter by lazy { wireMoshi.adapter(GitCheckoutRequest::class.java) }
    private val fsBrowseAdapter by lazy { wireMoshi.adapter(FsBrowseResult::class.java) }
    private val fsTreeAdapter by lazy { wireMoshi.adapter(FsDirectoryTree::class.java) }
    private val fsEntryAdapter by lazy { wireMoshi.adapter(FsTreeEntry::class.java) }
    private val fileSearchAdapter by lazy { wireMoshi.adapter(FileSearchResult::class.java) }
    private val slashCommandAdapter by lazy { wireMoshi.adapter(SlashCommandInfo::class.java) }
    private val authStatusAdapter by lazy { wireMoshi.adapter(AuthStatusResponse::class.java) }
    private val loginUrlAdapter by lazy { wireMoshi.adapter(OAuthLoginUrlResponse::class.java) }
    private val githubStatusAdapter by lazy { wireMoshi.adapter(console.v1.GitHubAuthStatus::class.java) }
    private val providerCredentialsAdapter by lazy { wireMoshi.adapter(console.v1.ProviderCredentialStatuses::class.java) }
    private val setProviderCredentialAdapter by lazy { wireMoshi.adapter(console.v1.SetProviderCredentialResponse::class.java) }
    private val integrationsAdapter by lazy { wireMoshi.adapter(console.v1.IntegrationsResponse::class.java) }
    private val deviceAdapter by lazy { wireMoshi.adapter(DeviceDescriptor::class.java) }
    private val forwardedPortAdapter by lazy { wireMoshi.adapter(ForwardedPort::class.java) }
    private val deviceDiagnosticsAdapter by lazy { wireMoshi.adapter(DeviceDiagnostics::class.java) }
    private val deviceActionAdapter by lazy { wireMoshi.adapter(DeviceActionRequest::class.java) }
    private val assistSearchAdapter by lazy { wireMoshi.adapter(AssistFileSearchResponse::class.java) }
    private val fsFileContentAdapter by lazy { wireMoshi.adapter(FsFileContent::class.java) }
    private val writeFileAdapter by lazy { wireMoshi.adapter(WriteFileRequest::class.java) }
    private val createDirAdapter by lazy { wireMoshi.adapter(CreateDirRequest::class.java) }
    private val pairDeviceAdapter by lazy { wireMoshi.adapter(console.v1.PairDeviceRequest::class.java) }
    private val pairDeviceResponseAdapter by lazy { wireMoshi.adapter(console.v1.PairDeviceResponse::class.java) }
    private val pairingGrantAdapter by lazy { wireMoshi.adapter(console.v1.PairingGrantRequest::class.java) }
    private val pairingGrantResponseAdapter by lazy { wireMoshi.adapter(console.v1.PairingGrantResponse::class.java) }
    private val serverDevicesAdapter by lazy { wireMoshi.adapter(console.v1.ServerDevicesResponse::class.java) }
    private fun enc(v: String): String = URLEncoder.encode(v, "UTF-8")

    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean): List<SessionHeader> {
        val params = mutableMapOf<String, String?>()
        if (cwd != null) params["cwd"] = cwd
        if (projectId != null) params["projectId"] = projectId
        if (onlyDeleted) params["onlyDeleted"] = "true"
        val raw = http.get("/api/sessions", params)
        val element = http.unwrap(raw, JsonElement.serializer(), "list sessions")
        val array = element as? JsonArray ?: throw ApiException("Failed to list sessions")
        return array.map { item ->
            sessionAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to list sessions")
        }
    }

    override suspend fun getSession(id: String, limit: Int?, before: Long?): SessionDetailResponse {
        val params = mutableMapOf<String, String?>()
        if (limit != null) params["limit"] = limit.toString()
        if (before != null) params["before"] = before.toString()
        val raw = http.get("/api/sessions/${enc(id)}", params)
        // Envelope stays kotlinx; header and messages decode through the
        // shared schema, scalars read field by field.
        val obj = http.unwrap(raw, JsonObject.serializer(), "load session")
        val header = obj["header"]?.let { sessionAdapter.fromJson(it.toString()) }
            ?: throw ApiException("Failed to load session")
        // Rows are the canonical oneof shape; unparseable rows are skipped.
        val messages = obj["messages"]?.jsonArray?.mapNotNull { item ->
            runCatching { messageMoshi.fromJson(item.toString())?.toUi() }.getOrNull()
        } ?: emptyList()
        return SessionDetailResponse(
            header = header,
            messages = messages,
            hasMore = obj["hasMore"]?.jsonPrimitive?.booleanOrNull ?: false,
            nextCursor = obj["nextCursor"]?.jsonPrimitive?.longOrNull,
        )
    }

    override suspend fun createSession(payload: CreateSessionDto): SessionHeader {
        val raw = http.post("/api/sessions", http.encodeBody(CreateSessionDto.serializer(), payload))
        val element = http.unwrap(raw, JsonElement.serializer(), "create session")
        return sessionAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to create session")
    }

    override suspend fun updateSession(id: String, payload: UpdateSessionDto): SessionHeader {
        var body = http.encodeBody(UpdateSessionDto.serializer(), payload)
        if (payload.clearProject) {
            val obj = kotlinx.serialization.json.Json.parseToJsonElement(body) as kotlinx.serialization.json.JsonObject
            body = kotlinx.serialization.json.JsonObject(obj + ("projectId" to kotlinx.serialization.json.JsonNull)).toString()
        }
        val raw = http.patch("/api/sessions/${enc(id)}", body)
        val element = http.unwrap(raw, JsonElement.serializer(), "update session")
        return sessionAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to update session")
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

    override suspend fun forkSession(id: String, messageId: String): SessionHeader {
        val body = buildJsonObject { put("messageId", messageId) }.toString()
        val raw = http.post("/api/sessions/${enc(id)}/fork", body)
        val element = http.unwrap(raw, JsonElement.serializer(), "fork session")
        return sessionAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to fork session")
    }

    override suspend fun getTodos(id: String): List<TodoItem> {
        val raw = http.get("/api/sessions/${enc(id)}/todos")
        val element = http.unwrap(raw, JsonElement.serializer(), "get session todos")
        val array = element as? JsonArray ?: throw ApiException("Failed to get session todos")
        return array.map { item ->
            todoAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to get session todos")
        }
    }

    override suspend fun getSubagents(id: String): List<SubagentInfo> {
        val raw = http.get("/api/sessions/${enc(id)}/subagents")
        val element = http.unwrap(raw, JsonElement.serializer(), "get session subagents")
        val array = element as? JsonArray ?: throw ApiException("Failed to get session subagents")
        return array.map { item ->
            subagentAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to get session subagents")
        }
    }

    override suspend fun getBackgroundWork(id: String): List<BackgroundWork> {
        val raw = http.get("/api/sessions/${enc(id)}/background-work")
        return http.unwrap(raw, ListSerializer(BackgroundWork.serializer()), "get background work")
    }

    override suspend fun cancelBackgroundWork(id: String, workId: String): BackgroundWork {
        val raw = http.post("/api/sessions/${enc(id)}/background-work/${enc(workId)}/cancel")
        return http.unwrap(raw, BackgroundWork.serializer(), "cancel background work")
    }

    override suspend fun getBackgroundWorkOutput(
        id: String,
        workId: String,
        cursor: Int,
        limit: Int,
    ): BackgroundWorkOutput {
        val raw = http.get(
            "/api/sessions/${enc(id)}/background-work/${enc(workId)}/output",
            mapOf("cursor" to cursor.toString(), "limit" to limit.toString()),
        )
        return http.unwrap(raw, BackgroundWorkOutput.serializer(), "get background work output")
    }

    override suspend fun getChanges(id: String): List<SessionFileChange> {
        val raw = http.get("/api/sessions/${enc(id)}/changes")
        val element = http.unwrap(raw, JsonElement.serializer(), "get session changes")
        val array = element as? JsonArray ?: throw ApiException("Failed to get session changes")
        return array.map { item ->
            sessionChangeAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to get session changes")
        }
    }

    override suspend fun getChangeDiff(id: String, path: String, turnIndex: Int): String? {
        val raw = http.get(
            "/api/sessions/${enc(id)}/changes/diff",
            mapOf("path" to path, "turnIndex" to turnIndex.toString()),
        )
        val element = http.unwrap(raw, JsonElement.serializer(), "load change diff")
        return (element as? JsonObject)?.get("diffText")?.jsonPrimitive?.contentOrNull
    }

    override suspend fun markChangeReviewed(id: String, path: String, turnIndex: Int, reviewed: Boolean) {
        val body = buildJsonObject {
            put("path", path)
            put("turnIndex", turnIndex)
            put("reviewed", reviewed)
        }.toString()
        http.post("/api/sessions/${enc(id)}/changes/reviewed", body)
    }



    override suspend fun getQueuedPrompt(id: String): QueuedPrompt? {
        val raw = http.get("/api/sessions/${enc(id)}/queue")
        val element = http.unwrap(raw, JsonElement.serializer(), "get queued prompt")
        if (element is JsonNull) return null
        return queuedPromptAdapter.fromJson(element.toString())
    }

    override suspend fun queuePrompt(id: String, payload: RunPromptDto): QueuedPrompt {
        val raw = http.post("/api/sessions/${enc(id)}/queue", http.encodeBody(RunPromptDto.serializer(), payload))
        val element = http.unwrap(raw, JsonElement.serializer(), "queue prompt")
        return queuedPromptAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to queue prompt")
    }

    override suspend fun editQueuedPrompt(id: String, payload: RunPromptDto): QueuedPrompt {
        val raw = http.put("/api/sessions/${enc(id)}/queue", http.encodeBody(RunPromptDto.serializer(), payload))
        val element = http.unwrap(raw, JsonElement.serializer(), "edit queued prompt")
        return queuedPromptAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to edit queued prompt")
    }

    override suspend fun clearQueuedPrompt(id: String): Boolean {
        val raw = http.delete("/api/sessions/${enc(id)}/queue")
        val element = http.unwrap(raw, JsonElement.serializer(), "clear queued prompt")
        return (element as? JsonObject)?.get("deleted")?.jsonPrimitive?.booleanOrNull ?: false
    }

    override suspend fun steerRun(id: String, payload: RunPromptDto) {
        val raw = http.post("/api/sessions/${enc(id)}/steer", http.encodeBody(RunPromptDto.serializer(), payload))
        ensureOk(raw, "steer run")
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
        // Envelope-or-raw stays kotlinx; only the payload items are Wire types.
        val element = http.unwrapOrRaw(raw, JsonElement.serializer(), "list projects")
        val array = element as? JsonArray ?: throw ApiException("Failed to list projects")
        return array.map { item ->
            projectAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to list projects")
        }
    }

    override suspend fun addProject(path: String): ProjectInfo {
        val body = buildJsonObject { put("path", path) }.toString()
        val raw = http.post("/api/projects", body)
        val element = http.unwrapOrRaw(raw, JsonElement.serializer(), "add project")
        return projectAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to add project")
    }

    override suspend fun deleteProject(projectId: String) {
        val raw = http.delete("/api/projects/${enc(projectId)}")
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete project")
    }

    override suspend fun getFsBrowse(path: String?, showHidden: Boolean): FsBrowseResult {
        val raw = http.get(
            "/api/fs/browse",
            mapOf(
                "path" to path,
                "hidden" to if (showHidden) "true" else null,
            ),
        )
        // Envelope stays kotlinx; only the data payload is a Wire type.
        val data = http.unwrapOrRaw(raw, JsonObject.serializer(), "browse fs")
        return fsBrowseAdapter.fromJson(data.toString())
            ?: throw ApiException("Failed to browse fs")
    }

    override suspend fun getFsTree(path: String?): FsDirectoryTree {
        val raw = http.get("/api/fs/tree", mapOf("path" to path))
        val data = http.unwrapOrRaw(raw, JsonObject.serializer(), "load fs tree")
        return fsTreeAdapter.fromJson(data.toString())
            ?: throw ApiException("Failed to load fs tree")
    }

    override suspend fun getFsEntries(path: String, depth: Int): List<FsTreeEntry> {
        val raw = http.get("/api/fs/entries", mapOf("path" to path, "depth" to depth.toString()))
        val element = http.unwrapOrRaw(raw, JsonElement.serializer(), "load fs entries")
        val array = element as? JsonArray ?: throw ApiException("Failed to load fs entries")
        return array.map { item ->
            fsEntryAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to load fs entries")
        }
    }

    override suspend fun searchFiles(root: String, query: String, limit: Int, includeDirs: Boolean): List<FileSearchResult> {
        val raw = http.get(
            "/api/fs/search",
            mapOf("root" to root, "q" to query, "limit" to limit.toString(), "includeDirs" to includeDirs.toString()),
        )
        return try {
            val element = http.unwrapOrRaw(raw, JsonElement.serializer(), "search files")
            val array = element as? JsonArray ?: return emptyList()
            array.mapNotNull { item -> fileSearchAdapter.fromJson(item.toString()) }
        } catch (_: Exception) {
            emptyList()
        }
    }

    override suspend fun readFile(path: String): FsFileContent {
        val raw = http.get("/api/fs/file", mapOf("path" to path))
        val data = http.unwrapOrRaw(raw, JsonObject.serializer(), "read file")
        return fsFileContentAdapter.fromJson(data.toString())
            ?: throw ApiException("Failed to read file")
    }

    override suspend fun writeFile(path: String, content: String) {
        // Same bytes as the old hand-built object: path, content.
        val body = writeFileAdapter.toJson(WriteFileRequest(path = path, content = content))
        val raw = http.post("/api/fs/file", body)
        http.unwrapOrRaw(raw, JsonElement.serializer(), "write file")
    }

    override suspend fun deleteFile(path: String) {
        val raw = http.delete("/api/fs/file", mapOf("path" to path))
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete file")
    }

    override suspend fun createDir(path: String) {
        val body = createDirAdapter.toJson(CreateDirRequest(path = path))
        val raw = http.post("/api/fs/dir", body)
        http.unwrapOrRaw(raw, JsonElement.serializer(), "create dir")
    }

    override suspend fun deleteDir(path: String) {
        val raw = http.delete("/api/fs/dir", mapOf("path" to path))
        http.unwrapOrRaw(raw, JsonElement.serializer(), "delete dir")
    }

    // These three deliberately do NOT swallow failures into a null/empty result.
    // A null here used to be indistinguishable from "no changes", so a dead backend
    // rendered as "The working tree is clean." Callers (ChangesScreen,
    // GitRepository) already handle the throw.


    override suspend fun getGitStatus(path: String): GitStatusSummary? {
        val raw = http.get("/api/git/status", mapOf("path" to path))
        val element = http.unwrap(raw, JsonElement.serializer(), "load git status")
        if (element is JsonNull) return null
        return gitStatusAdapter.fromJson(element.toString())
    }

    override suspend fun listBranches(repoPath: String): GitBranchesResponse? {
        val raw = http.get("/api/git/branches", mapOf("path" to repoPath))
        val element = http.unwrap(raw, JsonElement.serializer(), "list git branches")
        if (element is JsonNull) return null
        return gitBranchesAdapter.fromJson(element.toString())
    }

    override suspend fun getSessionContext(sessionId: String): console.v1.ContextSnapshot? {
        val raw = http.get("/api/sessions/${enc(sessionId)}/context")
        val element = http.unwrap(raw, JsonElement.serializer(), "get session context")
        if (element is JsonNull) return null
        return contextAdapter.fromJson(element.toString())
    }

    override suspend fun attachWorktree(sessionId: String, branch: String?, baseBranch: String?): SessionHeader {
        val body = kotlinx.serialization.json.buildJsonObject {
            if (!branch.isNullOrBlank()) put("branch", kotlinx.serialization.json.JsonPrimitive(branch))
            if (!baseBranch.isNullOrBlank()) put("baseBranch", kotlinx.serialization.json.JsonPrimitive(baseBranch))
        }.toString()
        val raw = http.post("/api/sessions/${enc(sessionId)}/worktree", body)
        val element = http.unwrap(raw, JsonElement.serializer(), "attach worktree")
        return sessionAdapter.fromJson(element.toString()) ?: throw ApiException("Failed to attach worktree")
    }

    override suspend fun checkoutBranch(repoPath: String, branch: String) {
        // Same bytes as the old hand-built object: path, branch.
        val body = gitCheckoutAdapter.toJson(GitCheckoutRequest(path = repoPath, branch = branch))
        val raw = http.post("/api/git/checkout", body)
        ensureOk(raw, "checkout branch")
    }

    override suspend fun getProviders(): List<ProviderCatalogEntry> {
        val raw = http.get("/api/providers")
        // Envelope stays kotlinx; the catalog items are Wire types.
        val element = http.unwrap(raw, JsonElement.serializer(), "list providers")
        val array = element as? JsonArray ?: throw ApiException("Failed to list providers")
        return array.map { item ->
            providerCatalogAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to list providers")
        }
    }

    override suspend fun getProviderModels(providerId: String): List<Model> {
        val raw = http.get("/api/providers/${enc(providerId)}/models")
        val element = http.unwrapOrRaw(raw, JsonElement.serializer(), "list provider models")
        return providerModelsAdapter.fromJson(element.toString())?.models.orEmpty()
    }

    override suspend fun getApprovalModes(): List<ApprovalModeOption> {
        val raw = http.get("/api/config/approval-modes")
        return http.unwrap(raw, ListSerializer(ApprovalModeOption.serializer()), "list approval modes")
    }

    override suspend fun pair(request: PairingRequest): PairingData {
        val body = pairDeviceAdapter.toJson(request)
        val raw = http.post("/api/auth/pair", body)
        return http.unwrapOrRawAdapter(raw, pairDeviceResponseAdapter, "pair with server")
    }

    override suspend fun pairWithUrl(baseUrl: String, request: PairingRequest): PairingData {
        val body = pairDeviceAdapter.toJson(request)
        val raw = http.postAbsolute(baseUrl, "/api/auth/pair", body)
        return http.unwrapOrRawAdapter(raw, pairDeviceResponseAdapter, "pair with server")
    }

    override suspend fun createPairingGrant(request: PairingGrantRequest): PairingGrantData {
        val body = pairingGrantAdapter.toJson(request)
        val raw = http.post("/api/auth/pairing-grants", body)
        return http.unwrapOrRawAdapter(raw, pairingGrantResponseAdapter, "create pairing grant")
    }

    override suspend fun listAuthDevices(): List<DeviceInfo> {
        val raw = http.get("/api/auth/devices")
        return http.unwrapOrRawAdapter(raw, serverDevicesAdapter, "list devices").devices
    }

    override suspend fun revokeAuthDevice(id: String) {
        val raw = http.delete("/api/auth/devices/$id")
        http.unwrap(raw, JsonElement.serializer(), "revoke device")
    }

    override suspend fun getAuthStatus(): AuthStatusResponse {
        val raw = http.get("/api/auth/status")
        return http.unwrapOrRawAdapter(raw, authStatusAdapter, "get auth status")
    }

    override suspend fun getLoginUrl(payload: OAuthLoginUrlDto): OAuthLoginUrlResponse {
        val raw = http.post("/api/auth/login/url", http.encodeBody(OAuthLoginUrlDto.serializer(), payload))
        return http.unwrapOrRawAdapter(raw, loginUrlAdapter, "get login url")
    }

    override suspend fun handleCallback(payload: OAuthCallbackDto) {
        val raw = http.post("/api/auth/login/callback", http.encodeBody(OAuthCallbackDto.serializer(), payload))
        http.unwrap(raw, JsonElement.serializer(), "handle auth callback")
    }

    override suspend fun connectGitHub(payload: GitHubTokenDto): console.v1.GitHubAuthStatus {
        val raw = http.post("/api/auth/github/pat", http.encodeBody(GitHubTokenDto.serializer(), payload))
        return http.unwrapOrRawAdapter(raw, githubStatusAdapter, "connect GitHub")
    }

    override suspend fun getGitHubStatus(): console.v1.GitHubAuthStatus {
        val raw = http.get("/api/auth/github/status")
        return http.unwrapOrRawAdapter(raw, githubStatusAdapter, "get GitHub status")
    }

    override suspend fun disconnectGitHub() {
        val raw = http.post("/api/auth/github/logout")
        http.unwrapOrRaw(raw, JsonElement.serializer(), "disconnect GitHub")
    }

    override suspend fun getProviderCredentials(): Map<String, ProviderCredentialStatus> {
        val raw = http.get("/api/providers/credentials")
        val statuses = http.unwrapOrRawAdapter(raw, providerCredentialsAdapter, "get provider credentials")
        // Wire synthesizes the map field as a nullable property even though
        // protojson omits an empty map entirely, so an absent map reads as
        // null rather than {}.
        return statuses.providers ?: emptyMap()
    }

    override suspend fun setProviderCredential(provider: String, apiKey: String): SetProviderCredentialResponse {
        // The key is sent once and dropped: the response reports only whether
        // one is configured and where it came from.
        val body = http.encodeBody(ProviderCredentialRequest.serializer(), ProviderCredentialRequest(apiKey))
        val raw = http.put("/api/providers/$provider/credential", body)
        return http.unwrapOrRawAdapter(raw, setProviderCredentialAdapter, "set provider credential")
    }

    override suspend fun clearProviderCredential(provider: String): SetProviderCredentialResponse {
        val raw = http.delete("/api/providers/$provider/credential")
        return http.unwrapOrRawAdapter(raw, setProviderCredentialAdapter, "clear provider credential")
    }

    override suspend fun getIntegrations(): console.v1.IntegrationsResponse {
        val raw = http.get("/api/integrations")
        return http.unwrapOrRawAdapter(raw, integrationsAdapter, "get integrations")
    }

    override suspend fun installGh(): console.v1.IntegrationsResponse {
        val raw = http.post("/api/integrations/gh/install")
        return http.unwrapOrRawAdapter(raw, integrationsAdapter, "install GitHub CLI")
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
        val items = http.unwrapOrRaw(raw, JsonArray.serializer(), "list slash commands")
        return items.mapNotNull { slashCommandAdapter.fromJson(it.toString()) }
    }

    override suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): AssistFileSearchResponse {
        val path = if (sessionId != null) "/api/assist/${enc(sessionId)}/search" else "/api/assist/search"
        val params = mutableMapOf("q" to query)
        if (root != null) params["root"] = root
        val raw = http.get(path, params)
        return http.unwrapOrRawAdapter(raw, assistSearchAdapter, "assist search")
    }

    // Same reasoning as the git endpoints: an empty/null usage result must mean
    // "genuinely no usage", never "the request failed". UsageRepository already
    // surfaces thrown ApiExceptions through UsageState.error.

    override suspend fun getProviderUsage(providerId: String): UsageReport? {
        val raw = http.get("/api/providers/${enc(providerId)}/usage")
        // Nullable: a logged-out provider comes back as JSON null.
        val element = http.unwrap(raw, JsonElement.serializer(), "get usage for $providerId")
        if (element is JsonNull) return null
        return usageReportAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to get usage for $providerId")
    }

    override suspend fun getAllUsage(): Map<String, UsageReport?> {
        val raw = http.get("/api/usage")
        val obj = http.unwrap(raw, JsonObject.serializer(), "get all usage")
        return obj.mapValues { (_, value) ->
            if (value is JsonNull) null
            else usageReportAdapter.fromJson(value.toString())
        }
    }

    override suspend fun listFavorites(): List<ModelFavorite> {
        val raw = http.get("/api/model-favorites")
        val array = http.unwrap(raw, JsonArray.serializer(), "list model favorites")
        return array.map { element ->
            favoriteAdapter.fromJson(element.toString())
                ?: throw ApiException("Failed to list model favorites")
        }
    }

    override suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean) {
        // Canonical bytes match the old hand-built object: provider, modelId, favorite.
        val body = setFavoriteAdapter.toJson(
            SetFavoriteRequest(
                provider = favorite.provider,
                model_id = favorite.model_id,
                favorite = isFavorite,
            ),
        )
        val raw = http.put("/api/model-favorites", body)
        http.unwrap(raw, JsonElement.serializer(), "update model favorite")
    }

    override suspend fun getSettings(): ConsoleSettings {
        val raw = http.get("/api/settings")
        // Envelope stays kotlinx; only the data payload is a Wire type.
        val data = http.unwrap(raw, JsonObject.serializer(), "load settings")
        return settingsAdapter.fromJson(data.toString())
            ?: throw ApiException("Failed to load settings")
    }

    override suspend fun updateModelRoles(roles: Map<String, String?>): ConsoleSettings {
        // Request stays hand-built: a null reference clears the role, so it
        // must be sent as JSON null rather than dropped from the patch.
        val body = buildJsonObject {
            putJsonObject("modelRoles") {
                for ((role, ref) in roles) {
                    if (ref == null) put(role, JsonNull) else put(role, ref)
                }
            }
        }.toString()
        val raw = http.patch("/api/settings", body)
        val data = http.unwrap(raw, JsonObject.serializer(), "save model roles")
        return settingsAdapter.fromJson(data.toString())
            ?: throw ApiException("Failed to save model roles")
    }

    // mcp
    private val mcpServerAdapter = wireMoshi.adapter(McpServerStatus::class.java)

    override suspend fun listMcpServers(): List<McpServerStatus> {
        val raw = http.get("/api/mcp/servers")
        // Envelope stays kotlinx; the rows are Wire types.
        val element = http.unwrap(raw, JsonElement.serializer(), "list MCP servers")
        val array = element as? JsonArray ?: throw ApiException("Failed to list MCP servers")
        return array.map { item ->
            mcpServerAdapter.fromJson(item.toString())
                ?: throw ApiException("Failed to list MCP servers")
        }
    }

    override suspend fun saveMcpServer(payload: McpSavePayload): McpServerStatus {
        val raw = http.post("/api/mcp/servers", http.encodeBody(McpSavePayload.serializer(), payload))
        val element = http.unwrap(raw, JsonElement.serializer(), "save MCP server")
        return mcpServerAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to save MCP server")
    }

    override suspend fun updateMcpServer(id: String, payload: McpSavePayload): McpServerStatus {
        val raw = http.put("/api/mcp/servers/${enc(id)}", http.encodeBody(McpSavePayload.serializer(), payload))
        val element = http.unwrap(raw, JsonElement.serializer(), "update MCP server")
        return mcpServerAdapter.fromJson(element.toString())
            ?: throw ApiException("Failed to update MCP server")
    }

    override suspend fun deleteMcpServer(id: String) {
        val raw = http.delete("/api/mcp/servers/${enc(id)}")
        ensureOk(raw, "delete MCP server")
    }

    override suspend fun connectMcpServer(id: String, redirectUri: String?) {
        val body = if (redirectUri != null) {
            buildJsonObject { put("redirectUri", redirectUri) }.toString()
        } else {
            null
        }
        val raw = http.post("/api/mcp/servers/${enc(id)}/connect", body)
        ensureOk(raw, "connect MCP server")
    }

    override suspend fun disconnectMcpServer(id: String) {
        val raw = http.post("/api/mcp/servers/${enc(id)}/disconnect")
        ensureOk(raw, "disconnect MCP server")
    }

    override suspend fun forwardMcpOAuthCallback(id: String, payload: McpOAuthCallbackPayload) {
        val raw = http.post(
            "/api/mcp/servers/${enc(id)}/oauth/callback",
            http.encodeBody(McpOAuthCallbackPayload.serializer(), payload),
        )
        ensureOk(raw, "forward MCP OAuth callback")
    }

    override suspend fun getDevices(): List<DeviceDescriptor> {
        val raw = http.get("/api/devices")
        val items = http.unwrapOrRaw(raw, JsonArray.serializer(), "load devices")
        return items.mapNotNull { deviceAdapter.fromJson(it.toString()) }
    }

    override suspend fun getDeviceDiagnostics(): DeviceDiagnostics {
        val raw = http.get("/api/devices/diagnostics")
        return http.unwrapOrRawAdapter(raw, deviceDiagnosticsAdapter, "load device diagnostics")
    }

    override suspend fun bootDevice(id: String, platform: String) {
        val raw = http.post("/api/devices/${enc(id)}/boot?platform=${enc(platform)}", "{}")
        ensureOk(raw, "boot device")
    }

    override suspend fun shutdownDevice(id: String, platform: String) {
        val raw = http.post("/api/devices/${enc(id)}/shutdown?platform=${enc(platform)}", "{}")
        ensureOk(raw, "shutdown device")
    }

    override suspend fun interactDevice(id: String, platform: String, action: DeviceActionRequest) {
        // Wire type: encoded through Moshi, not kotlinx.
        val body = deviceActionAdapter.toJson(action)
        val raw = http.post("/api/devices/${enc(id)}/interact?platform=${enc(platform)}", body)
        ensureOk(raw, "interact with device")
    }

    override suspend fun getPorts(projectId: String?): List<ForwardedPort> {
        val raw = http.get("/api/ports", mapOf("projectId" to projectId))
        val items = http.unwrapOrRaw(raw, JsonArray.serializer(), "load ports")
        return items.mapNotNull { forwardedPortAdapter.fromJson(it.toString()) }
    }

    override suspend fun forwardPort(port: Int, projectId: String?): ForwardedPort {
        val body = buildJsonObject {
            put("port", port)
            if (projectId != null) put("projectId", projectId)
        }.toString()
        val raw = http.post("/api/ports/forward", body)
        return http.unwrapOrRawAdapter(raw, forwardedPortAdapter, "forward port")
    }

    override suspend fun unforwardPort(port: Int, projectId: String?) {
        val raw = http.delete("/api/ports/$port", mapOf("projectId" to projectId))
        ensureOk(raw, "unforward port")
    }

    private fun ensureOk(raw: String, action: String) {
        try {
            val root = ConsoleJson.parseToJsonElement(raw) as? JsonObject
            val success = (root?.get("success") as? JsonPrimitive)?.booleanOrNull
            if (success == false) {
                throw ApiException((root.get("error") as? JsonPrimitive)?.contentOrNull ?: "Failed to $action")
            }
        } catch (e: ApiException) {
            throw e
        } catch (_: Exception) {
        }
    }

}
