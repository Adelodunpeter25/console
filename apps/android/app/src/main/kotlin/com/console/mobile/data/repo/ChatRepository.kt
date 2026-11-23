package com.console.mobile.data.repo

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.PendingQuestion
import com.console.mobile.core.chat.abortSessionRun
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.finalizeSessionRun
import com.console.mobile.core.chat.isAbortError
import com.console.mobile.core.chat.newMessageId
import com.console.mobile.core.chat.toChatSnapshot
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.TranscriptTurn

import com.console.mobile.core.util.mentionPaths
import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AskQuestionRequest
import com.console.mobile.data.model.AssistantMessage
import com.console.mobile.data.model.BackgroundWorkOutputFrame
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.RunPromptDto
import com.console.mobile.data.model.SessionStatus
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.SubagentInfo
import com.console.mobile.data.model.TextPart
import console.v1.TodoItem
import com.console.mobile.data.model.UserMessage
import com.console.mobile.data.model.encodeBase64
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.stream.ChatStreamClient
import com.console.mobile.data.stream.RunStreamController
import java.util.UUID
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

class ChatRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val streamClient: ChatStreamClient,
    private val chats: ChatStateHolder,
    private val sessions: SessionStateHolder,
    private val projectState: com.console.mobile.data.store.ProjectStateHolder? = null,
    private val providerRepo: ProviderRepository? = null,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    private val controllers = mutableMapOf<String, RunStreamController>()

    /** Prompts shown only after the server accepts the run POST; see startRun. */
    private val pendingRuns = mutableMapOf<String, PendingRun>()

    private class PendingRun(
        val message: UserMessage,
        val prompt: String,
        val attachments: List<ImageAttachment>,
    )



    fun snapshot(sessionId: String) = toChatSnapshot(chats.get(sessionId))

    fun setInput(sessionId: String, value: String) {
        chats.update(sessionId) {
            it.copy(
                input = value,
                draftUpdatedAt = if (value.trim().isNotEmpty() || it.attachments.isNotEmpty()) System.currentTimeMillis() else null,
            )
        }
        scheduleDraftPush(sessionId)
    }

    /** Append a quoted reply to the draft. Reads the current input here, so callers need not capture it. */
    fun appendQuote(sessionId: String, quote: String) {
        val current = chats.get(sessionId).input
        val next = if (current.isBlank()) {
            quote
        } else {
            val separator = if (current.endsWith("\n")) "\n" else "\n\n"
            "$current$separator$quote"
        }
        setInput(sessionId, next)
    }

    // --- Synced drafts -----------------------------------------------------
    // Text only on this slice. The server keeps one draft per chat and
    // replaces it whole, so the file comments another device staged are held
    // here from the last pull and echoed back on every push; dropping them
    // would delete them from the desktop.
    private val serverDrafts = mutableMapOf<String, console.v1.ChatDraft>()
    private val draftPushJobs = mutableMapOf<String, kotlinx.coroutines.Job>()

    private fun draftKey(sessionId: String): String =
        if (com.console.mobile.core.chat.isNewChatId(sessionId)) FLOATING_DRAFT_KEY else sessionId

    /** Pull the chat's synced draft on open; adopt it when it is newer than ours. */
    fun loadDraft(sessionId: String) {
        val key = draftKey(sessionId)
        scope.launch {
            try {
                val remote = withContext(Dispatchers.IO) { api.getDraft(key) } ?: return@launch
                adoptDraft(sessionId, remote)
            } catch (_: Exception) {
            }
        }
    }

    private fun adoptDraft(sessionId: String, remote: console.v1.ChatDraft) {
        val key = draftKey(sessionId)
        serverDrafts[key] = remote
        val local = chats.get(sessionId)
        if (local.running || remote.updated_at <= (local.draftUpdatedAt ?: 0L)) return
        chats.update(sessionId) {
            it.copy(
                input = remote.text,
                draftUpdatedAt = if (remote.text.trim().isNotEmpty() || remote.floating != null) remote.updated_at else null,
            )
        }
        if (com.console.mobile.core.chat.isNewChatId(sessionId)) {
            remote.floating?.let { applyFloatingChoices(sessionId, it) }
        }
    }

    private fun applyFloatingChoices(sessionId: String, choices: console.v1.FloatingDraftChoices) {
        val current = sessions.getView(sessionId)
        val worktreePending = choices.branch_mode == "new_worktree"
        sessions.setView(
            sessionId,
            current.copy(
                projectId = choices.project_id.takeIf { it.isNotBlank() } ?: current.projectId,
                sessionProvider = choices.provider.takeIf { it.isNotBlank() } ?: current.sessionProvider,
                sessionModelId = choices.model_id.takeIf { it.isNotBlank() } ?: current.sessionModelId,
                thinkingLevel = choices.thinking_level.takeIf { it.isNotBlank() } ?: current.thinkingLevel,
                approvalMode = choices.approval_mode.takeIf { it.isNotBlank() } ?: current.approvalMode,
                worktreePending = worktreePending,
                worktreeBase = choices.worktree_base.takeIf { it.isNotBlank() },
                worktreeBranch = if (worktreePending) "New worktree" else choices.branch_name.takeIf { it.isNotBlank() },
            ),
        )
    }

    private fun buildFloatingChoices(sessionId: String): console.v1.FloatingDraftChoices {
        val view = sessions.getView(sessionId)
        val (branchMode, branchName) = when {
            view.worktreePending -> "new_worktree" to ""
            !view.worktreeBranch.isNullOrBlank() -> "from_branch" to view.worktreeBranch
            else -> "project" to ""
        }
        return console.v1.FloatingDraftChoices(
            project_id = view.projectId.orEmpty(),
            branch_mode = branchMode,
            branch_name = branchName,
            worktree_base = view.worktreeBase.orEmpty(),
            provider = view.sessionProvider.orEmpty(),
            model_id = view.sessionModelId.orEmpty(),
            thinking_level = view.thinkingLevel.orEmpty(),
            approval_mode = view.approvalMode,
        )
    }

    /**
     * Fold a bootstrap response's side panels into chat state with the same
     * rules the individual loaders use: the server is authoritative for
     * todos, suggestions, background work and the queue, and a draft only
     * wins when it is newer than the local one. Messages and changes are
     * applied by [SessionRepository].
     */
    fun applyBootstrap(sessionId: String, b: com.console.mobile.data.model.SessionBootstrapResponse) {
        setTodoItems(sessionId, b.todos)
        setSuggestions(sessionId, if (chats.get(sessionId).running) emptyList() else b.suggestions)
        val bootstrapQuestions = b.pendingQuestions
            .filter { it.request_id.isNotBlank() && it.question.isNotBlank() }
            .map { PendingQuestion(it.toUi()) }
        chats.update(sessionId) {
            val mergedQuestions = (it.pendingQuestions + bootstrapQuestions)
                .distinctBy { q -> q.request.requestId }
            it.copy(
                backgroundWork = if (it.backgroundWork == b.backgroundWork) it.backgroundWork else b.backgroundWork,
                queuedPrompt = b.queuedPrompts.firstOrNull(),
                pendingQuestions = if (bootstrapQuestions.isNotEmpty()) mergedQuestions else it.pendingQuestions,
            )
        }
        if (bootstrapQuestions.isNotEmpty()) {
            sessions.setStatus(sessionId, SessionStatus.NeedsAttention)
        }
        b.draft?.let { adoptDraft(sessionId, it) }
    }

    fun scheduleDraftPush(sessionId: String) {
        val key = draftKey(sessionId)
        draftPushJobs.remove(key)?.cancel()
        draftPushJobs[key] = scope.launch {
            kotlinx.coroutines.delay(DRAFT_PUSH_DEBOUNCE_MS)
            pushDraft(sessionId)
        }
    }

    /** Push immediately, e.g. when leaving the chat; also used after a send clears the composer. */
    fun flushDraft(sessionId: String) {
        val key = draftKey(sessionId)
        draftPushJobs.remove(key)?.cancel()
        scope.launch { pushDraft(sessionId) }
    }

    fun clearFloatingDraft() {
        draftPushJobs.remove(FLOATING_DRAFT_KEY)?.cancel()
        scope.launch {
            val draft = console.v1.ChatDraft(
                session_id = FLOATING_DRAFT_KEY,
                text = "",
                updated_at = System.currentTimeMillis(),
            )
            try {
                val result = withContext(Dispatchers.IO) { api.putDraft(draft) }
                result.draft?.let { serverDrafts[FLOATING_DRAFT_KEY] = it }
            } catch (_: Exception) {
            }
        }
    }

    private suspend fun pushDraft(sessionId: String) {
        val key = draftKey(sessionId)
        val isFloating = com.console.mobile.core.chat.isNewChatId(sessionId)
        val state = chats.get(sessionId)
        val text = state.input
        val held = serverDrafts[key]
        val draft = console.v1.ChatDraft(
            session_id = key,
            text = text,
            mentions = if (held?.text == text) held.mentions else emptyList(),
            comments = held?.comments ?: emptyList(),
            updated_at = state.draftUpdatedAt ?: System.currentTimeMillis(),
            floating = if (isFloating) buildFloatingChoices(sessionId) else null,
        )
        try {
            val result = withContext(Dispatchers.IO) { api.putDraft(draft) }
            val stored = result.draft
            if (stored != null) serverDrafts[key] = stored
            if (!result.applied && stored != null && chats.get(sessionId).input == text) {
                chats.update(sessionId) {
                    it.copy(
                        input = stored.text,
                        draftUpdatedAt = if (stored.text.trim().isNotEmpty() || stored.floating != null) stored.updated_at else null,
                    )
                }
                if (isFloating) {
                    stored.floating?.let { applyFloatingChoices(sessionId, it) }
                }
            }
        } catch (_: Exception) {
        }
    }

    fun addAttachments(sessionId: String, attachments: List<ImageAttachment>) {
        if (attachments.isEmpty()) return
        chats.update(sessionId) { current ->
            current.copy(
                attachments = current.attachments + attachments,
                draftUpdatedAt = System.currentTimeMillis(),
            )
        }
    }

    fun removeAttachment(sessionId: String, index: Int) {
        chats.update(sessionId) { current ->
            val next = current.attachments.filterIndexed { i, _ -> i != index }
            current.copy(
                attachments = next,
                draftUpdatedAt = if (current.input.trim().isNotEmpty() || next.isNotEmpty()) System.currentTimeMillis() else null,
            )
        }
    }

    fun clearAttachments(sessionId: String) {
        chats.update(sessionId) { current ->
            current.copy(
                attachments = emptyList(),
                draftUpdatedAt = if (current.input.trim().isNotEmpty()) System.currentTimeMillis() else null,
            )
        }
    }

    fun setTodoItems(sessionId: String, items: List<TodoItem>) {
        chats.update(sessionId) { it.copy(todoItems = items) }
    }

    fun setSubagents(sessionId: String, subagents: List<SubagentInfo>) {
        chats.update(sessionId) { it.copy(subagents = subagents) }
    }

    /** Refresh the server-backed task list whenever a chat is opened. */
    fun loadTodos(sessionId: String) {
        scope.launch {
            try {
                // An in-memory chat state survives navigation, so a non-empty list
                // is not evidence that it is still current. The server response is
                // authoritative here, including an empty response after tasks are
                // completed or removed.
                val todos = withContext(Dispatchers.IO) { api.getTodos(sessionId) }
                setTodoItems(sessionId, todos)
            } catch (_: Exception) {
            }
        }
    }

    fun setSuggestions(sessionId: String, items: List<String>) {
        chats.update(sessionId) { if (it.suggestions == items) it else it.copy(suggestions = items) }
    }

    /**
     * Fetch the chat's stored follow-ups: on open, and after the notification
     * stream reconnects, when a push may have been missed. The server is
     * authoritative, so an empty reply clears stale chips. A chat that started
     * running meanwhile keeps none.
     */
    fun loadSuggestions(sessionId: String) {
        scope.launch {
            try {
                val items = withContext(Dispatchers.IO) { api.getSuggestions(sessionId) }
                setSuggestions(sessionId, if (chats.get(sessionId).running) emptyList() else items)
            } catch (_: Exception) {
            }
        }
    }

    /** A chip tap: a normal turn (or a queued one mid-run) that leaves the typed draft and attachments in the composer. */
    fun sendSuggestion(sessionId: String, text: String) {
        val prompt = text.trim()
        if (prompt.isEmpty()) return
        // Mid-run it is staged for after the turn, like any prompt sent then.
        if (chats.get(sessionId).running) {
            setSuggestions(sessionId, emptyList())
            queuePrompt(sessionId, prompt, clearComposer = false)
            return
        }
        startRun(sessionId, prompt, attachments = emptyList(), contextFiles = emptyList(), clearComposer = false)
    }

    /** Fill the composer with a suggestion for editing, replacing its draft. */
    fun editSuggestion(sessionId: String, text: String) {
        val prompt = text.trim()
        if (prompt.isEmpty()) return
        chats.update(sessionId) { current ->
            current.copy(
                input = prompt,
                attachments = emptyList(),
                draftUpdatedAt = System.currentTimeMillis(),
                focusRequest = current.focusRequest + 1,
            )
        }
        setSuggestions(sessionId, emptyList())
    }

    fun loadSubagents(sessionId: String) {
        scope.launch {
            try {
                val subagents = withContext(Dispatchers.IO) { api.getSubagents(sessionId) }
                if (subagents.isNotEmpty()) {
                    setSubagents(sessionId, subagents)
                }
            } catch (_: Exception) {
            }
        }
    }

    /** Hydrate the chat's detached shell list; the chip filters it to running shells. */
    fun loadBackgroundWork(sessionId: String) {
        scope.launch {
            try {
                val work = withContext(Dispatchers.IO) { api.getBackgroundWork(sessionId) }
                chats.update(sessionId) { if (it.backgroundWork == work) it else it.copy(backgroundWork = work) }
            } catch (_: Exception) {
            }
        }
    }

    /** Stop one detached shell and fold the server's cancelled record back into state. */
    fun cancelBackgroundWork(sessionId: String, workId: String) {
        scope.launch {
            try {
                val work = withContext(Dispatchers.IO) { api.cancelBackgroundWork(sessionId, workId) }
                chats.update(sessionId) { state ->
                    state.copy(backgroundWork = state.backgroundWork.map { if (it.id == work.id) work else it })
                }
            } catch (_: Exception) {
                // A completed shell's cancel can 404; refresh so the chip drops it.
                loadBackgroundWork(sessionId)
            }
        }
    }

    /**
     * Live output for the background-shell sheet, pushed by the server as the
     * shell writes. Emits one frame per update and completes when the shell is
     * no longer running; it throws if the stream cannot be opened.
     */
    fun backgroundWorkOutputStream(sessionId: String, workId: String): Flow<BackgroundWorkOutputFrame> =
        streamClient.shellOutput(apiClient.baseUrl, sessionId, workId, apiClient.authToken)

    /** Load any staged next-turn prompt into state on session open. */
    fun loadQueuedPrompt(sessionId: String) {
        scope.launch {
            try {
                val queued = withContext(Dispatchers.IO) { api.getQueuedPrompt(sessionId) }
                chats.update(sessionId) { it.copy(queuedPrompt = queued) }
            } catch (_: Exception) {
            }
        }
    }

    /** One-off fetch so the ring has a value before the first live update. */
    fun loadContext(sessionId: String) {
        scope.launch {
            try {
                val snap = withContext(Dispatchers.IO) { api.getSessionContext(sessionId) } ?: return@launch
                chats.update(sessionId) { if (it.context == null) it.copy(context = snap) else it }
            } catch (_: Exception) {
            }
        }
    }

    fun loadTurns(sessionId: String, turns: List<TranscriptTurn>) {
        if (turns.isEmpty()) return
        chats.update(sessionId) { current ->
            if (current.running) {
                val serverIds = turns.mapTo(HashSet()) { it.id }
                val localTail = current.turns.lastOrNull()?.takeIf { it.id !in serverIds && it.run?.status == RunStatus.Working }
                val merged = if (localTail != null && turns.lastOrNull()?.run?.status != RunStatus.Working) {
                    turns + localTail
                } else {
                    turns
                }
                current.copy(turns = merged)
            } else {
                current.copy(
                    turns = turns,
                    streamingText = "",
                    streamingThinking = "",
                    activeToolCalls = emptyList(),
                    pendingQuestions = emptyList(),
                )
            }
        }
    }

    fun setPagination(sessionId: String, hasMore: Boolean, nextCursor: Long?) {
        chats.update(sessionId) { it.copy(hasMoreMessages = hasMore, nextCursor = nextCursor) }
    }

    fun setLoadingOlder(sessionId: String, loading: Boolean) {
        chats.update(sessionId) { it.copy(loadingOlder = loading) }
    }

    fun prependTurns(sessionId: String, older: List<TranscriptTurn>) {
        if (older.isEmpty()) return
        chats.update(sessionId) { current ->
            val seen = current.turns.mapTo(HashSet()) { it.id }
            val fresh = older.filterNot { it.id in seen }
            if (fresh.isEmpty()) current else current.copy(turns = fresh + current.turns)
        }
    }

    fun fetchToolResult(sessionId: String, messageId: String, callId: String) {
        scope.launch {
            try {
                val full = withContext(Dispatchers.IO) { api.getToolResult(sessionId, messageId, callId) }
                chats.update(sessionId) { current ->
                    val updatedTurns = current.turns.map { turn ->
                        val run = turn.run ?: return@map turn
                        var changed = false
                        val updatedEvents = run.events.map { ev ->
                            if (ev is com.console.mobile.core.chat.ActivityEvent.ToolCallEvent && ev.call.id == callId) {
                                changed = true
                                ev.copy(result = full)
                            } else ev
                        }
                        if (changed) turn.copy(run = run.copy(events = updatedEvents)) else turn
                    }
                    current.copy(turns = updatedTurns)
                }
            } catch (_: Exception) {
            }
        }
    }

    fun handleEvent(sessionId: String, event: AgentSessionEvent) {
        if (event.type == "askQuestion") {
            sessions.setStatus(sessionId, SessionStatus.NeedsAttention)
        }
        when (event.type) {
            "askQuestion" -> {
                val req = parseQuestion(event) ?: return
                chats.update(sessionId) {
                    if (it.pendingQuestions.any { q -> q.request.requestId == req.requestId }) it
                    else it.copy(pendingQuestions = it.pendingQuestions + PendingQuestion(req))
                }
                return
            }
            "questionResolved" -> {
                val resolvedId = event.requestId?.takeIf { it.isNotBlank() } ?: return
                var remainingEmpty = false
                var stillRunning = false
                chats.update(sessionId) {
                    val next = it.pendingQuestions.filterNot { q -> q.request.requestId == resolvedId }
                    remainingEmpty = next.isEmpty()
                    stillRunning = it.running
                    it.copy(pendingQuestions = next)
                }
                if (remainingEmpty && stillRunning) {
                    sessions.setStatus(sessionId, SessionStatus.Working)
                }
                return
            }
            "sessionTitleUpdated" -> {
                val title = event.title?.trim()
                if (!title.isNullOrEmpty()) {
                    projectState?.updateSessionTitle(sessionId, title)
                }
                return
            }
            "sessionApprovalModeUpdated" -> {
                val mode = event.approvalMode?.trim()
                if (!mode.isNullOrEmpty()) {
                    sessions.updateApprovalMode(sessionId, mode)
                }
                return
            }
            else -> chats.update(sessionId) { applyChatEvent(it, event) }
        }
    }

    /** Send a prompt — appends the user bubble then opens POST /run SSE (or queues if running). */
    fun sendMessage(sessionId: String, promptOverride: String? = null) {
        val session = chats.get(sessionId)
        val prompt = (promptOverride ?: session.input).trim()
        if (prompt.isEmpty()) return
        if (session.running) {
            queuePrompt(sessionId, prompt)
            return
        }
        startRun(sessionId, prompt, session.attachments, contextFiles = mentionPaths(prompt), clearComposer = true)
    }

    /**
     * Open POST /run with `prompt` after appending the user bubble. Callers
     * must have already verified the session is idle. A queued prompt's
     * send-now path passes [clearComposer] = false so the composer's freshly
     * typed note survives the click.
     */
    private fun startRun(
        sessionId: String,
        prompt: String,
        attachments: List<ImageAttachment>,
        contextFiles: List<String>,
        clearComposer: Boolean,
    ) {
        val view = sessions.getView(sessionId)

        if (attachments.isNotEmpty() && providerRepo?.supportsImages(view.sessionProvider, view.sessionModelId) == false) {
            chats.update(sessionId) { current ->
                val errMsg = AssistantMessage(
                    id = newMessageId(),
                    createdAt = System.currentTimeMillis(),
                    content = listOf(TextPart("Error: The selected model '${view.sessionModelId}' does not support image attachments.")),
                )
                val turns = current.turns.toMutableList()
                if (turns.isEmpty()) {
                    turns.add(TranscriptTurn(id = newMessageId(), createdAt = System.currentTimeMillis(), responses = listOf(errMsg)))
                } else {
                    turns[turns.lastIndex] = turns.last().let { it.copy(responses = it.responses + errMsg) }
                }
                current.copy(turns = turns)
            }
            return
        }

        // ImageAttachment holds raw bytes; the transcript and the request body
        // are the boundaries where base64 is actually needed.
        val userMessageAttachments = attachments.map {
            com.console.mobile.data.model.ImagePart(data = encodeBase64(it.bytes), mimeType = it.mimeType)
        }

        val userMessage = UserMessage(
            id = newMessageId(),
            createdAt = System.currentTimeMillis(),
            content = prompt,
            attachments = userMessageAttachments,
            contextFiles = contextFiles,
            approvalMode = view.approvalMode.takeIf { it.isNotBlank() },
        )
        // The bubble is held back until the server accepts the run POST — a
        // rejected prompt (e.g. 409 right after a stop) must not render as a
        // sent message, or it silently vanishes on the next restart.
        pendingRuns[sessionId] = PendingRun(userMessage, prompt, attachments)
        projectState?.unsettleSession(sessionId)

        chats.update(sessionId) {
            it.copy(
                input = if (clearComposer) "" else it.input,
                draftUpdatedAt = if (clearComposer) null else it.draftUpdatedAt,
                streamingText = "",
                streamingThinking = "",
                activeToolCalls = emptyList(),
                suggestions = emptyList(),
                running = true,
                attachments = if (clearComposer) emptyList() else it.attachments,
            )
        }
        if (clearComposer) {
            flushDraft(sessionId)
        }
        sessions.setStatus(sessionId, SessionStatus.Working)

        val body = RunPromptDto(
            prompt = prompt,
            modelId = view.sessionModelId,
            provider = view.sessionProvider,
            approvalMode = view.approvalMode.takeIf { it.isNotBlank() },
            thinkingLevel = providerRepo?.validThinkingLevel(view.sessionProvider, view.sessionModelId, view.thinkingLevel) ?: view.thinkingLevel.takeIf { providerRepo == null },
            attachments = attachments,
            contextFiles = contextFiles,
        )
        val bodyJson = ConsoleJson.encodeToString(RunPromptDto.serializer(), body)
        val controller = getOrCreate(sessionId, bodyJson)
        try {
            controller.startRun(streamClient)
        } catch (e: Exception) {
            val msg = e.message ?: "Failed to send message. Is the backend running?"
            pendingRuns.remove(sessionId)
            restoreFailedSend(sessionId)
            if (!isAbortError(msg)) markError(sessionId, msg)
            finalize(sessionId, !isAbortError(msg))
        }
    }

    /** A run POST that never reached the server: give the prompt back to the composer. */
    private fun restoreFailedSend(sessionId: String) {
        val pending = pendingRuns.remove(sessionId) ?: return
        chats.update(sessionId) { s ->
            if (s.input.isBlank()) {
                s.copy(
                    input = pending.prompt,
                    attachments = pending.attachments,
                    draftUpdatedAt = System.currentTimeMillis(),
                )
            } else s
        }
    }

    /** User stop — kill stream/timers, abort server run, mark aborted. */
    fun abort(sessionId: String) {
        // Remove (not just cancel): a cancelled controller stays inert and a
        // reused one would swallow the next send's events.
        controllers.remove(sessionId)?.cancel()
        scope.launch {
            try {
                withContext(Dispatchers.IO) { api.abortRun(sessionId) }
            } catch (_: Exception) {
            }
            chats.update(sessionId) { abortSessionRun(it) }
            sessions.setStatus(sessionId, SessionStatus.Done)
        }
    }

    /** Stage prompt to run right after the active turn settles (POST /api/sessions/:id/queue). */
    fun queuePrompt(sessionId: String, promptText: String? = null, clearComposer: Boolean = true) {
        val session = chats.get(sessionId)
        val prompt = (promptText ?: session.input).trim()
        if (prompt.isEmpty()) return
        val view = sessions.getView(sessionId)
        // A chip carries no attachments of its own, and must not take the draft's.
        val attachments = if (clearComposer) session.attachments else emptyList()

        if (clearComposer) {
            // Clear local input so user can type subsequent notes
            chats.update(sessionId) {
                it.copy(
                    input = "",
                    draftUpdatedAt = null,
                    attachments = emptyList(),
                )
            }
            flushDraft(sessionId)
        }
        projectState?.unsettleSession(sessionId)

        val body = RunPromptDto(
            prompt = prompt,
            modelId = view.sessionModelId,
            provider = view.sessionProvider,
            approvalMode = view.approvalMode.takeIf { it.isNotBlank() },
            thinkingLevel = providerRepo?.validThinkingLevel(view.sessionProvider, view.sessionModelId, view.thinkingLevel) ?: view.thinkingLevel.takeIf { providerRepo == null },
            attachments = attachments,
            contextFiles = mentionPaths(prompt),
        )
        scope.launch {
            try {
                val queued = withContext(Dispatchers.IO) { api.queuePrompt(sessionId, body) }
                chats.update(sessionId) { it.copy(queuedPrompt = it.queuedPrompt ?: queued) }
            } catch (e: Exception) {
                // Restore draft on failure. A chip never took the draft, so
                // there is nothing to give back.
                if (clearComposer) {
                    chats.update(sessionId) {
                        it.copy(
                            input = prompt,
                            attachments = attachments,
                        )
                    }
                }
            }
        }
    }

    /** Discard staged prompt from server (DELETE /api/sessions/:id/queue). */
    fun deleteQueuedPrompt(sessionId: String) {
        val q = chats.get(sessionId).queuedPrompt ?: return
        scope.launch {
            try {
                withContext(Dispatchers.IO) { api.deleteQueuedPrompt(sessionId, q.id) }
                chats.update(sessionId) { it.copy(queuedPrompt = null) }
                loadQueuedPrompt(sessionId)
            } catch (_: Exception) {}
        }
    }

    /** Pop queued prompt back into composer for editing. */
    fun editQueuedPrompt(sessionId: String) {
        val session = chats.get(sessionId)
        val q = session.queuedPrompt ?: return
        scope.launch {
            try {
                withContext(Dispatchers.IO) { api.deleteQueuedPrompt(sessionId, q.id) }
            } catch (_: Exception) {}
            loadQueuedPrompt(sessionId)
            chats.update(sessionId) { current ->
                val attachments = q.attachments.map {
                    ImageAttachment(
                        id = UUID.randomUUID().toString(),
                        bytes = android.util.Base64.decode(it.data_, android.util.Base64.DEFAULT),
                        mimeType = it.mime_type,
                    )
                }
                current.copy(
                    queuedPrompt = null,
                    input = q.prompt,
                    attachments = attachments,
                    draftUpdatedAt = System.currentTimeMillis(),
                )
            }
        }
    }

    /** Steer active turn immediately with the queued prompt (POST /api/sessions/:id/steer). */
    fun steerQueuedPrompt(sessionId: String) {
        val session = chats.get(sessionId)
        val q = session.queuedPrompt ?: return
        if (!session.running && controllers[sessionId]?.isActive != true) {
            // Idle: there is no run to pivot, so the send button is send-now —
            // start a fresh run with the staged prompt. The server's staged
            // copy is cleared first, or the new run would fire it again after
            // settling; the composer's fresh note survives (clearComposer =
            // false).
            chats.update(sessionId) { it.copy(queuedPrompt = null) }
            val attachments = q.attachments.map {
                ImageAttachment(
                    id = UUID.randomUUID().toString(),
                    bytes = android.util.Base64.decode(it.data_, android.util.Base64.DEFAULT),
                    mimeType = it.mime_type,
                )
            }
            scope.launch {
                try {
                    withContext(Dispatchers.IO) { api.deleteQueuedPrompt(sessionId, q.id) }
                } catch (_: Exception) {}
                startRun(sessionId, q.prompt, attachments, contextFiles = q.context_files, clearComposer = false)
            }
            return
        }
        val view = sessions.getView(sessionId)
        val attachments = q.attachments.map {
            ImageAttachment(
                id = UUID.randomUUID().toString(),
                bytes = android.util.Base64.decode(it.data_, android.util.Base64.DEFAULT),
                mimeType = it.mime_type,
            )
        }
        val body = RunPromptDto(
            prompt = q.prompt,
            modelId = q.model_id ?: view.sessionModelId,
            provider = q.provider ?: view.sessionProvider,
            approvalMode = q.approval_mode ?: view.approvalMode.takeIf { it.isNotBlank() },
            thinkingLevel = q.thinking_level ?: view.thinkingLevel.takeIf { providerRepo == null },
            attachments = attachments,
            contextFiles = q.context_files,
        )
        scope.launch {
            try {
                // The server still holds this card; take it out first, or it
                // would run again after the steered prompt.
                withContext(Dispatchers.IO) { api.deleteQueuedPrompt(sessionId, q.id) }
                try {
                    withContext(Dispatchers.IO) { api.steerRun(sessionId, body) }
                } catch (e: Exception) {
                    // The run may have ended mid-flight: put the card back.
                    withContext(Dispatchers.IO) { api.queuePrompt(sessionId, body) }
                    throw e
                }
                chats.update(sessionId) { it.copy(queuedPrompt = null) }
                loadQueuedPrompt(sessionId)
            } catch (_: Exception) {}
        }
    }

    /**
     * Attach to a server-side active run (re-attach). Called when entering a
     * session whose server status is working but no local stream exists.
     */
    fun attachServerRun(sessionId: String) {
        val current = chats.get(sessionId)
        if (current.running || controllers[sessionId]?.isActive == true) return
        chats.update(sessionId) { it.copy(running = true) }
        sessions.setStatus(sessionId, SessionStatus.Working)
        try {
            // since=0 replays the whole current run buffer, which is what an
            // attach wants — a run this device never saw start has no local
            // cursor. The reducer is idempotent on turn and tool-call id, so the
            // overlap with the loaded history is dropped rather than duplicated.
            getOrCreate(sessionId, "{}").attach(0, streamClient)
        } catch (_: Exception) {
            finalize(sessionId, false)
        }
    }

    fun answerQuestion(sessionId: String, requestId: String, answer: JsonElement) {
        scope.launch {
            try {
                withContext(Dispatchers.IO) {
                    api.answerQuestion(sessionId, com.console.mobile.data.model.AnswerQuestionDto(requestId, answer))
                }
            } catch (_: Exception) {
            }
            chats.update(sessionId) { s ->
                s.copy(pendingQuestions = s.pendingQuestions.filterNot { it.request.requestId == requestId })
            }
        }
    }

    fun clear(sessionId: String) = chats.clear(sessionId)

    /**
     * The session no longer exists: stop its stream and drop everything held
     * locally, including the unsent draft, so the chat leaves no row behind.
     */
    fun discard(sessionId: String) {
        controllers.remove(sessionId)?.cancel()
        pendingRuns.remove(sessionId)
        chats.remove(sessionId)
    }

    private fun getOrCreate(sessionId: String, bodyJson: String): RunStreamController {
        // A finished or user-cancelled controller stays inert (it drops every
        // event), so it must be replaced rather than reused for a new send.
        controllers[sessionId]?.takeIf { it.isActive }?.let { return it }
        val controller = RunStreamController(sessionId, object : RunStreamController.Deps {
            override fun handleEvent(event: AgentSessionEvent) = this@ChatRepository.handleEvent(sessionId, event)
            override fun markError(message: String) = markError(sessionId, message)
            override fun finalize(hadError: Boolean) {
                this@ChatRepository.finalize(sessionId, hadError)
                controllers.remove(sessionId)
            }
            override fun onRunOpened() = this@ChatRepository.onRunOpened(sessionId)
            override fun baseUrl(): String = apiClient.baseUrl
            override fun authToken(): String? = apiClient.authToken
            override fun runBodyJson(): String = bodyJson
            override fun scope(): CoroutineScope = this@ChatRepository.scope
        })
        controllers[sessionId] = controller
        return controller
    }

    private fun onRunOpened(sessionId: String) {
        val pending = pendingRuns.remove(sessionId) ?: return
        val now = pending.message.createdAt ?: System.currentTimeMillis()
        val turn = TranscriptTurn(
            id = newMessageId(),
            createdAt = now,
            userMessage = pending.message,
            run = RunActivityState(runId = newMessageId(), startedAt = now, status = RunStatus.Working),
        )
        chats.update(sessionId) { it.copy(turns = it.turns + turn) }
    }

    private fun markError(sessionId: String, msg: String) {
        chats.update(sessionId) { s ->
            val errMsg = com.console.mobile.data.model.AssistantMessage(
                id = newMessageId(),
                createdAt = System.currentTimeMillis(),
                content = listOf(com.console.mobile.data.model.TextPart("Error: $msg")),
            )
            val turns = s.turns.toMutableList()
            if (turns.isEmpty()) {
                turns.add(TranscriptTurn(id = newMessageId(), createdAt = System.currentTimeMillis(), responses = listOf(errMsg)))
            } else {
                turns[turns.lastIndex] = turns.last().let { it.copy(responses = it.responses + errMsg) }
            }
            s.copy(
                turns = turns,
                streamingText = "",
                streamingThinking = "",
            )
        }
    }

    private fun finalize(sessionId: String, hadError: Boolean) {
        // A run that never started (retries exhausted) still holds its prompt back.
        restoreFailedSend(sessionId)
        chats.update(sessionId) { finalizeSessionRun(it, hadError) }
        sessions.setStatus(sessionId, if (hadError) SessionStatus.NeedsAttention else SessionStatus.Done)
    }

    // Interactive request payloads are console.v1 (args as JSON bytes),
    // decoded with Moshi; the hand types stay as the UI render models.
    private val wireMoshi: Moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
    private val askAdapter = wireMoshi.adapter(console.v1.AskQuestionRequest::class.java)

    private fun console.v1.AskQuestionRequest.toUi(): AskQuestionRequest = AskQuestionRequest(
        requestId = request_id,
        question = question,
        options = options,
        isMultiSelect = is_multi_select,
        // The server always sets skippable, but the tool input defaults
        // it to true, so default to true when the key is missing.
        skippable = skippable ?: true,
        batchId = batch_id,
    )

    private fun parseQuestion(event: AgentSessionEvent): AskQuestionRequest? {
        val raw = event.request?.toString() ?: return null
        val req = runCatching { askAdapter.fromJson(raw) }
            .onFailure { android.util.Log.w("ChatRepository", "Failed to parse askQuestion payload", it) }
            .getOrNull() ?: return null
        if (req.request_id.isBlank() || req.question.isBlank()) return null
        return req.toUi()
    }

    private companion object {
        const val DRAFT_PUSH_DEBOUNCE_MS = 800L
        const val FLOATING_DRAFT_KEY = "__floating__"
    }
}
