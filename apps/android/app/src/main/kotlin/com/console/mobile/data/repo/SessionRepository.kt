package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.data.model.CreateSessionDto
import com.console.mobile.data.model.SessionDetailResponse
import com.console.mobile.data.model.SessionPrompt
import console.v1.SessionHeader
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.store.SessionViewState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Everything about chats: the list Home shows, one chat's detail and paging,
 * and the actions on a chat (create, rename, settle, read, delete, fork).
 * Projects are [ProjectRepository]'s. There is one session list, held in
 * [ProjectStateHolder.state]; nothing here keeps a second copy.
 */
class SessionRepository(
    private val api: ConsoleApi,
    private val projectState: ProjectStateHolder,
    private val sessions: SessionStateHolder,
    private val appState: AppStateHolder,
    private val chats: ChatStateHolder,
    private val chatRepo: ChatRepository,
    private val persistence: com.console.mobile.data.local.ChatPersistence? = null,
    /** Called once a session is gone for good, so chat state (and its draft) can be dropped. */
    private val onSessionGone: (String) -> Unit = {},
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    companion object {
        /**
         * Newest-page size on open. Matches the server's default page size
         * (`parsePageParams`) so the first paint covers the same span of
         * history the desktop app sees — a smaller window silently omits older
         * runs, and a run's "Worked for Ns" header only renders when its tool
         * calls are in the loaded messages.
         */
        const val FIRST_PAGE = 50
        /** Older-page size once the user scrolls to the top. */
        const val OLDER_PAGE = 50
    }

    fun loadSessions() {
        scope.launch {
            projectState.patchSessionsLoading(true)
            try {
                val list = withContext(Dispatchers.IO) { api.getSessions() }
                projectState.setSessions(list)
                sessions.setStatusesSeed(list.map { h ->
                    h.id to com.console.mobile.data.model.SessionStatus.fromValue(h.status)
                }.toMap())
            } catch (e: Exception) {
                projectState.patchSessionsLoading(false)
                projectState.setError(e.message ?: "Failed to load chat sessions")
            }
        }
    }

    fun loadDeletedSessions() {
        scope.launch {
            projectState.patchDeletedLoading(true)
            try {
                val list = withContext(Dispatchers.IO) { api.getSessions(onlyDeleted = true) }
                projectState.setDeleted(list)
            } catch (e: Exception) {
                projectState.patchDeletedLoading(false)
                projectState.setError(e.message ?: "Failed to load deleted chats")
            }
        }
    }

    suspend fun createSession(
        cwd: String? = null,
        projectId: String? = null,
        title: String? = "New Chat",
    ): SessionHeader = withContext(Dispatchers.IO) {
        val created = api.createSession(
            CreateSessionDto(
                cwd = cwd,
                projectId = projectId,
                title = title,
            ),
        )
        withContext(Dispatchers.Main.immediate) {
            projectState.prependSession(created)
            sessions.setStatus(created.id, com.console.mobile.data.model.SessionStatus.fromValue(created.status))
        }
        created
    }

    /** Open a new chat locally; nothing reaches the server until [commitNewChat]. */
    fun startNewChat(projectId: String?, cwd: String?): String {
        val id = com.console.mobile.core.chat.newChatId()
        val saved = persistence?.loadNewChatOptions()
        sessions.setView(
            id,
            SessionViewState(
                sessionModelId = saved?.model,
                sessionProvider = saved?.provider,
                sessionCwd = cwd ?: saved?.cwd,
                approvalMode = saved?.approvalMode ?: ApprovalMode.PlanMode.value,
                thinkingLevel = saved?.thinkingLevel,
                worktreePending = saved?.worktreePending == true,
                worktreeBranch = if (saved?.worktreePending == true) "New worktree" else null,
                projectId = projectId ?: saved?.projectId,
            ),
        )
        return id
    }

    fun discardNewChat(id: String) {
        if (!com.console.mobile.core.chat.isNewChatId(id)) return
        // The composer text stays in chats so it persists and returns on the next new chat.
        sessions.removeView(id)
    }

    /** Create the real session from a new chat's picks, move its composer over, and return the new id. */
    suspend fun commitNewChat(draftId: String): String {
        val view = sessions.getView(draftId)
        val created = withContext(Dispatchers.IO) {
            api.createSession(
                CreateSessionDto(
                    cwd = view.sessionCwd,
                    projectId = view.projectId,
                    modelId = view.sessionModelId,
                    provider = view.sessionProvider,
                    title = "New Chat",
                    approvalMode = view.approvalMode,
                    thinkingLevel = view.thinkingLevel,
                    worktree = if (view.worktreePending) com.console.mobile.data.model.CreateWorktreeDto() else null,
                ),
            )
        }
        withContext(Dispatchers.Main.immediate) {
            // The prompt is about to be sent, so the row reads "Generating title…" like desktop.
            projectState.prependSession(created.copy(message_count = 1))
            applyHeader(created.id, created)
            if (created.thinking_level.isNullOrBlank() && view.thinkingLevel != null) {
                sessions.setView(created.id, sessions.getView(created.id).copy(thinkingLevel = view.thinkingLevel))
            }
            val draft = chats.get(draftId)
            chats.update(created.id) {
                it.copy(input = draft.input, attachments = draft.attachments, draftUpdatedAt = draft.draftUpdatedAt)
            }
            discardNewChat(draftId)
            chats.remove(draftId)
            persistence?.saveNewChatOptions(null)
        }
        return created.id
    }

    private fun updateNewChat(id: String, dto: UpdateSessionDto): SessionHeader {
        val view = sessions.getView(id)
        val next = when {
            dto.clearProject -> view.copy(sessionCwd = null, projectId = null)
            dto.cwd != null -> view.copy(sessionCwd = dto.cwd, projectId = null)
            else -> view
        }.let {
            it.copy(
                sessionModelId = dto.modelId ?: it.sessionModelId,
                sessionProvider = dto.provider ?: it.sessionProvider,
                approvalMode = dto.approvalMode ?: it.approvalMode,
                thinkingLevel = dto.thinkingLevel ?: it.thinkingLevel,
            )
        }
        sessions.setView(id, next)
        saveNewChatOptions(next)
        return com.console.mobile.core.chat.createEphemeralDraftHeader(
            id, com.console.mobile.core.chat.createChatSessionState(), next.projectId, next.sessionCwd,
        )
    }

    fun requestNewChatWorktree(id: String) {
        val next = sessions.getView(id).copy(worktreePending = true, worktreeBranch = "New worktree")
        sessions.setView(id, next)
        saveNewChatOptions(next)
    }

    private fun saveNewChatOptions(v: SessionViewState) {
        persistence?.saveNewChatOptions(
            com.console.mobile.data.local.PersistedNewChatOptions(
                model = v.sessionModelId,
                provider = v.sessionProvider,
                approvalMode = v.approvalMode,
                thinkingLevel = v.thinkingLevel,
                worktreePending = v.worktreePending,
                projectId = v.projectId,
                cwd = v.sessionCwd,
            ),
        )
    }

    suspend fun updateSession(id: String, dto: UpdateSessionDto): SessionHeader = withContext(Dispatchers.IO) {
        if (com.console.mobile.core.chat.isNewChatId(id)) return@withContext updateNewChat(id, dto)
        val updated = api.updateSession(id, dto)
        withContext(Dispatchers.Main.immediate) {
            projectState.patchSession(id, updated)
        }
        updated
    }

    /**
     * Settle (or un-settle) a chat. The server refuses settling while a run is
     * active or a prompt is queued; the exception carries its message and the
     * local mark is only applied after the call succeeds.
     */
    suspend fun settleSession(id: String, settled: Boolean) = withContext(Dispatchers.IO) {
        if (settled) api.settleSession(id) else api.unsettleSession(id)
        withContext(Dispatchers.Main.immediate) {
            projectState.state.value.sessions.firstOrNull { it.id == id }?.let { header ->
                val patch = if (settled) {
                    // Pinned and settled are mutually exclusive: settling unpins.
                    header.copy(settled_at = System.currentTimeMillis(), pinned_at = null)
                } else {
                    header.copy(settled_at = null)
                }
                projectState.patchSession(id, patch)
            }
        }
    }

    /**
     * Tell the server this chat was shown, which clears its unread dot (and a
     * seen needs-attention) on every device. The local header is cleared first
     * so the dot does not linger until the next refresh, and the call is
     * skipped for a chat already read. Best-effort: a failure only means the
     * dot clears on the next refresh.
     */
    suspend fun markSessionRead(id: String, force: Boolean = false) {
        val header = projectState.state.value.sessions.firstOrNull { it.id == id } ?: return
        // `force` is for a run that just finished on screen: the local header is
        // stale (it has not been refreshed since the run ended), so it still
        // reads as already-read while the server has just marked it unread.
        if (!force && !header.unread && header.status != "needs_attention") return
        // A needs-attention chat with no run behind it settles to done on read.
        val settled = if (header.status == "needs_attention") "done" else header.status
        projectState.patchSession(id, header.copy(unread = false, status = settled))
        try {
            withContext(Dispatchers.IO) { api.markSessionRead(id) }
            if (force) loadSessions()
        } catch (e: Exception) {
            android.util.Log.d("ProjectRepository", "markSessionRead($id) failed", e)
        }
    }

    /**
     * Make a read chat unread again. The dot appears at once; a refusal from
     * the server (a running or settled chat) rolls it back and rethrows so the
     * caller can show the reason.
     */
    suspend fun markSessionUnread(id: String) {
        val header = projectState.state.value.sessions.firstOrNull { it.id == id } ?: return
        if (header.unread) return
        projectState.patchSession(id, header.copy(unread = true))
        try {
            withContext(Dispatchers.IO) { api.markSessionUnread(id) }
        } catch (e: Exception) {
            projectState.state.value.sessions.firstOrNull { it.id == id }?.let {
                projectState.patchSession(id, it.copy(unread = false))
            }
            throw e
        }
    }

    suspend fun deleteSession(id: String) = withContext(Dispatchers.IO) {
        try {
            api.deleteSession(id)
        } catch (e: com.console.mobile.data.api.ApiException) {
            // Already gone server-side (deleted from the desktop): the local row
            // and its draft are all that's left, and those still need clearing.
            if (e.code != "404") throw e
        }
        withContext(Dispatchers.Main.immediate) { forgetSession(id) }
    }

    /** The server deleted a chat (another device, or the settled-chat sweep). Main thread only. */
    fun onSessionDeletedRemotely(id: String) {
        if (projectState.state.value.sessions.any { it.id == id }) forgetSession(id)
    }

    private fun forgetSession(id: String) {
        projectState.removeSession(id)
        sessions.clearStatus(id)
        onSessionGone(id)
        if (appState.state.value.selectedSessionId == id) {
            appState.setSelectedSessionId(null)
            if (appState.state.value.activeTab == com.console.mobile.data.store.MobileTab.Chat) {
                appState.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
            }
        }
    }

    suspend fun restoreSession(id: String) = withContext(Dispatchers.IO) {
        api.restoreSession(id)
        withContext(Dispatchers.Main.immediate) {
            val restored = projectState.state.value.deletedSessions.firstOrNull { it.id == id }
            projectState.removeDeleted(id)
            if (restored != null) {
                projectState.prependSession(restored)
            }
        }
    }

    suspend fun permanentlyDeleteSession(id: String) = withContext(Dispatchers.IO) {
        api.permanentlyDeleteSession(id)
        withContext(Dispatchers.Main.immediate) {
            projectState.removeDeleted(id)
            onSessionGone(id)
        }
    }

    /** Fork a session through messageId; the new idle session is prepended like a fresh chat. */
    suspend fun forkSession(id: String, messageId: String): SessionHeader = withContext(Dispatchers.IO) {
        val forked = api.forkSession(id, messageId)
        withContext(Dispatchers.Main.immediate) {
            projectState.prependSession(forked)
            sessions.setStatus(forked.id, com.console.mobile.data.model.SessionStatus.fromValue(forked.status))
        }
        forked
    }


    /**
     * Sessions whose older-page fetch is in flight. Held in memory rather than
     * in [ChatSessionState] so a fast scroll can't enqueue a second request
     * before the `loadingOlder` flag round-trips through the state flow.
     */
    private val olderInFlight = mutableSetOf<String>()

    /**
     * Newest page for a session, plus the header. Suspends until the fetch
     * lands and returns the status the server reported, so a caller that has
     * to decide something from the status (attach to a live run) can read the
     * value this call produced instead of a stale one from the session list.
     * Fails soft: a failed fetch returns null and leaves existing state alone.
     */
    suspend fun loadDetail(sessionId: String, limit: Int = FIRST_PAGE): SessionHeader? {
        return try {
            val detail: SessionDetailResponse = withContext(Dispatchers.IO) { api.getSession(sessionId, limit, null) }
            applyHeader(sessionId, detail.header)
            chatRepo.loadMessages(sessionId, detail.messages)
            chatRepo.setPagination(sessionId, detail.hasMore, detail.nextCursor)
            chatRepo.loadQueuedPrompt(sessionId)
            detail.header
        } catch (e: Exception) {
            android.util.Log.w("SessionRepository", "loadDetail($sessionId) failed; chat keeps empty state", e)
            null
        }
    }

    /**
     * Fetch the next older page and prepend it. Mirrors the desktop's
     * `load_older_messages_for_pane`: no-op unless the session reports more
     * messages and nothing is already in flight, and a failure leaves the
     * cursor untouched so the user can retry by scrolling again.
     *
     * Returns whether the request was actually dispatched, so an auto-trigger
     * caller can hold off instead of spinning on a failing backend.
     */
    fun loadOlder(sessionId: String, limit: Int = OLDER_PAGE): Boolean {
        val before = chats.get(sessionId).nextCursor ?: return false
        if (!chats.get(sessionId).hasMoreMessages) return false
        if (!olderInFlight.add(sessionId)) return false
        chatRepo.setLoadingOlder(sessionId, true)
        scope.launch {
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, limit, before) }
                chatRepo.prependMessages(sessionId, detail.messages)
                chatRepo.setPagination(sessionId, detail.hasMore, detail.nextCursor)
            } catch (e: Exception) {
                android.util.Log.w("SessionRepository", "loadOlder($sessionId, before=$before) failed", e)
            } finally {
                olderInFlight.remove(sessionId)
                chatRepo.setLoadingOlder(sessionId, false)
            }
        }
        return true
    }

    /**
     * A chat's full prompt list as last fetched, with the newest loaded user
     * message it was fetched for. The list is refetched only when that message
     * changes (a new prompt), never just because the popup was reopened.
     */
    data class PromptList(val prompts: List<SessionPrompt>, val forLatestUserId: String?)

    private val _promptLists = MutableStateFlow<Map<String, PromptList>>(emptyMap())
    val promptLists: StateFlow<Map<String, PromptList>> = _promptLists.asStateFlow()

    /** Fetch the prompt list unless the cached one already covers [latestUserId]. Fails soft. */
    suspend fun ensurePrompts(sessionId: String, latestUserId: String?) {
        if (_promptLists.value[sessionId]?.forLatestUserId == latestUserId) return
        try {
            val prompts = withContext(Dispatchers.IO) { api.getSessionPrompts(sessionId) }
            _promptLists.update { it + (sessionId to PromptList(prompts, latestUserId)) }
        } catch (e: Exception) {
            android.util.Log.w("SessionRepository", "ensurePrompts($sessionId) failed", e)
        }
    }

    /**
     * Page older history in until at least [count] user messages are loaded.
     * Loaded history is always the newest contiguous slice, so the prompt that
     * is Nth from the newest is loaded exactly when N+1 user messages are.
     * Returns false when the start of history is reached or a fetch fails first.
     */
    suspend fun loadUntilUserMessages(sessionId: String, count: Int): Boolean {
        while (true) {
            val state = chats.get(sessionId)
            if (state.messages.count { it is com.console.mobile.data.model.UserMessage } >= count) return true
            val before = state.nextCursor
            if (!state.hasMoreMessages || before == null) return false
            // A scroll-triggered page may be in flight; wait for it, then re-check.
            if (!olderInFlight.add(sessionId)) {
                delay(50)
                continue
            }
            chatRepo.setLoadingOlder(sessionId, true)
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, OLDER_PAGE, before) }
                chatRepo.prependMessages(sessionId, detail.messages)
                chatRepo.setPagination(sessionId, detail.hasMore, detail.nextCursor)
            } catch (e: Exception) {
                android.util.Log.w("SessionRepository", "loadUntilUserMessages($sessionId, $count) failed", e)
                return false
            } finally {
                olderInFlight.remove(sessionId)
                chatRepo.setLoadingOlder(sessionId, false)
            }
        }
    }

    /** Load a session's file changes into state. Empty when none. */
    fun loadSessionChanges(sessionId: String) {
        scope.launch {
            try {
                val changes = withContext(Dispatchers.IO) { api.getChanges(sessionId) }
                sessions.setSessionChanges(sessionId, changes)
            } catch (_: Exception) {
            }
        }
    }

    /** Toggle a change's reviewed flag with an optimistic update + revert. */
    fun toggleChangeReviewed(sessionId: String, path: String, turnIndex: Int, reviewed: Boolean) {
        scope.launch {
            sessions.patchReviewed(sessionId, path, turnIndex, reviewed)
            try {
                withContext(Dispatchers.IO) { api.markChangeReviewed(sessionId, path, turnIndex, reviewed) }
            } catch (_: Exception) {
                sessions.patchReviewed(sessionId, path, turnIndex, !reviewed)
            }
        }
    }

    suspend fun loadChangeDiff(sessionId: String, path: String, turnIndex: Int): String? =
        withContext(Dispatchers.IO) {
            try {
                api.getChangeDiff(sessionId, path, turnIndex)
            } catch (_: Exception) {
                null
            }
        }

    /** Turn a message-less session into a worktree session; the header carries the new cwd/branch. */
    suspend fun attachWorktree(sessionId: String): SessionHeader {
        val header = withContext(Dispatchers.IO) { api.attachWorktree(sessionId, null, null) }
        applyHeader(sessionId, header)
        return header
    }

    /** Re-read one chat's header into the list and the chat's view state. */
    fun refreshHeader(sessionId: String) {
        if (com.console.mobile.core.chat.isNewChatId(sessionId)) return
        scope.launch {
            try {
                val detail = withContext(Dispatchers.IO) { api.getSession(sessionId, 1, null) }
                applyHeader(sessionId, detail.header)
            } catch (_: Exception) {
            }
        }
    }

    private fun applyHeader(sessionId: String, header: SessionHeader) {
        sessions.setView(
            sessionId,
            SessionViewState(
                sessionModelId = header.model_id,
                sessionProvider = header.provider,
                sessionCwd = header.cwd,
                approvalMode = header.approval_mode.ifBlank { ApprovalMode.PlanMode.value },
                thinkingLevel = header.thinking_level?.takeIf { it.isNotBlank() },
                worktreeBranch = header.worktree?.branch?.takeIf { it.isNotBlank() },
                projectId = header.project_id?.takeIf { it.isNotBlank() },
            ),
        )
        sessions.setStatus(sessionId, com.console.mobile.data.model.SessionStatus.fromValue(header.status))
        projectState.patchSession(sessionId, header)
    }
}
