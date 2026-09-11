package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.FsBrowseResult
import com.console.mobile.data.api.FsFileContent
import com.console.mobile.data.api.LoginUrlResult
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AgentSessionEvent
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
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.data.model.StreamPart
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TextPart
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.stream.ChatStreamClient
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.runTest
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/** In-memory ConsoleApi stub for repository tests (no network). */
private class FakeApi(
    var detail: SessionDetailResponse? = null,
    var aborted: Int = 0,
) : ConsoleApi {
    override suspend fun getSessions(cwd: String?, projectId: String?, onlyDeleted: Boolean): List<SessionHeader> = emptyList()
    override suspend fun getSession(id: String, limit: Int?, before: Long?): SessionDetailResponse =
        detail ?: SessionDetailResponse(header = SessionHeader(id, "T", "/tmp", null, "m", "codex", 1, 2))
    override suspend fun createSession(payload: CreateSessionDto): SessionHeader = throw UnsupportedOperationException()
    override suspend fun updateSession(id: String, payload: UpdateSessionDto): SessionHeader = throw UnsupportedOperationException()
    override suspend fun deleteSession(id: String) {}
    override suspend fun restoreSession(id: String) {}
    override suspend fun permanentlyDeleteSession(id: String) {}
    override suspend fun getTodos(id: String): List<TodoItem> = emptyList()
    override suspend fun getSubagents(id: String): List<SubagentInfo> = emptyList()
    override suspend fun getChanges(id: String): List<SessionFileChange> = emptyList()
    override suspend fun abortRun(sessionId: String) { aborted += 1 }
    override suspend fun answerQuestion(sessionId: String, payload: AnswerQuestionDto) {}
    override suspend fun approvePermission(sessionId: String, payload: ApproveToolPermissionDto): Boolean = payload.allow
    override suspend fun getProjects(): List<ProjectInfo> = emptyList()
    override suspend fun addProject(path: String): ProjectInfo = throw UnsupportedOperationException()
    override suspend fun deleteProject(projectId: String) {}
    override suspend fun getFsBrowse(path: String?): FsBrowseResult = FsBrowseResult("", null, emptyList())
    override suspend fun getFsTree(path: String?): List<FsTreeEntry> = emptyList()
    override suspend fun getFsEntries(path: String, depth: Int): List<FsTreeEntry> = emptyList()
    override suspend fun searchFiles(root: String, query: String, limit: Int, includeDirs: Boolean) = emptyList<com.console.mobile.data.model.FileSearchResult>()
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
    override suspend fun getAuthStatus(): AuthStatusShim = AuthStatusShim()
    override suspend fun getLoginUrl(payload: OAuthLoginUrlDto): LoginUrlResult = LoginUrlResult("", "", "")
    override suspend fun handleCallback(payload: OAuthCallbackDto) {}
    override suspend fun saveProjectId(provider: String, projectId: String?) {}
    override suspend fun listSlashCommands(sessionId: String?): List<SlashCommandInfo> = emptyList()
    override suspend fun assistSearchFiles(sessionId: String?, query: String, root: String?): FileSearchResponse =
        FileSearchResponse("", query, emptyList())
    override suspend fun getProviderUsage(providerId: String): UsageReport? = null
    override suspend fun getAllUsage(): Map<String, UsageReport?> = emptyMap()
    override suspend fun listFavorites(): List<ModelFavorite> = emptyList()
    override suspend fun setFavorite(favorite: ModelFavorite, isFavorite: Boolean) {}
}

@OptIn(ExperimentalCoroutinesApi::class)
class ChatRepositoryTest {
    private fun repo(scope: TestScope, api: FakeApi = FakeApi()): Triple<ChatRepository, ChatStateHolder, SessionStateHolder> {
        val chats = ChatStateHolder()
        val sessions = SessionStateHolder()
        val http = OkHttpClient.Builder().connectTimeout(5, TimeUnit.SECONDS).build()
        // ConsoleApiClient requires Android stores; bypass with a minimal fake via real class is
        // impossible here, so construct ChatStreamClient only and inject a stub client holder.
        // Instead exercise ChatRepository with a lightweight ConsoleApiClient substitute:
        // (ChatRepository only reads baseUrl/authToken — provide via a test double below.)
        val r = ChatRepository(api, TestApiClient(), ChatStreamClient(http), chats, sessions, scope)
        return Triple(r, chats, sessions)
    }

    @Test fun sendMessageAppendsUserBubbleAndRuns() = runTest {
        val (r, chats, sessions) = repo(this)
        chats.setInput("s1", "hello")
        r.sendMessage("s1")
        val s = chats.get("s1")
        assertTrue(s.running)
        assertEquals(1, s.messages.size)
        assertEquals(SessionStatus.Working, sessions.statuses.value["s1"])
        // Stop the background stream attempt.
        r.abort("s1")
    }

    @Test fun handleStreamPartAccumulates() = runTest {
        val (r, chats, _) = repo(this)
        r.handleEvent("s1", AgentSessionEvent(type = "turnStart", prompt = "hi"))
        r.handleEvent("s1", AgentSessionEvent(type = "modelStreamPart", part = StreamPart(text = "hel")))
        r.handleEvent("s1", AgentSessionEvent(type = "modelStreamPart", part = StreamPart(text = "lo")))
        assertEquals("hello", chats.get("s1").streamingText)
    }

    @Test fun loadMessagesSkipsWhileRunning() = runTest {
        val (r, chats, _) = repo(this)
        r.handleEvent("s1", AgentSessionEvent(type = "turnStart", prompt = "hi"))
        val msgs: List<AgentMessage> = listOf(
            AssistantMessage(content = listOf(TextPart("old"))),
        )
        r.loadMessages("s1", msgs)
        assertTrue(chats.get("s1").messages.isEmpty())
        r.abort("s1")
        r.loadMessages("s1", msgs)
        // abort is async; wait a tick by handling done through finalize path instead:
        // loadMessages still works once running=false — force via fresh holder below.
        val (r2, chats2, _) = repo(this)
        r2.loadMessages("s2", msgs)
        assertEquals(1, chats2.get("s2").messages.size)
    }

    // Minimal ConsoleApiClient stand-in: ChatRepository only needs baseUrl/authToken.
    private class TestApiClient : ConsoleApiClient(
        httpCallClient = OkHttpClient(),
        httpClient = OkHttpClient(),
        baseUrlOverride = "http://localhost:3000",
        authTokenOverride = null,
        hasAuthOverride = true,
    )
}
