package com.console.mobile.data.repo

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.PendingQuestion
import com.console.mobile.core.chat.abortSessionRun
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.ensureMessageIds
import com.console.mobile.core.chat.finalizeSessionRun
import com.console.mobile.core.chat.isAbortError
import com.console.mobile.core.chat.newMessageId
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.chat.toChatSnapshot
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.core.chat.mergeLiveHistory
import com.console.mobile.core.chat.prependOlderMessages
import com.console.mobile.core.chat.withLiveRun
import com.console.mobile.core.util.mentionPaths
import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.local.ChatPersistence
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
    private val persistence: ChatPersistence? = null,
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

    init {
        persistence?.let { p ->
            scope.launch(Dispatchers.IO) {
                val cached = p.load()
                if (cached.isNotEmpty()) {
                    // A cached draft can be a duplicate of a sent prompt when the
                    // app died before the cache caught up; validate against the
                    // server BEFORE injecting, so the old text never reaches the
                    // composer. One headers fetch covers every draft.
                    var validated = cached
                    try {
                        val headers = withContext(Dispatchers.IO) { api.getSessions() }.associateBy { it.id }
                        validated = cached.filter { (id, state) ->
                            val header = headers[id]
                            when {
                                header == null -> true
                                // Sent prompts persist messages, so a session
                                // whose server activity outdates the draft
                                // loses the cached input.
                                (header.message_count ?: 0) > 0 -> state.draftUpdatedAt == null ||
                                    header.updated_at <= state.draftUpdatedAt
                                else -> header.status != "working"
                            }
                        }.toMap()
                    } catch (_: Exception) {
                        // Offline: keep drafts as-is.
                    }
                    if (validated.size != cached.size) {
                        p.saveValidated(validated)
                    }
                    if (validated.isNotEmpty()) {
                        withContext(Dispatchers.Main.immediate) {
                            chats.setAll(validated)
                        }
                    }
                }
                chats.sessions.collect { current ->
                    p.scheduleSave(current)
                }
            }
        }
    }

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

    /** Pull the chat's synced draft on open; adopt it when it is newer than ours. */
    fun loadDraft(sessionId: String) {
        scope.launch {
            try {
                val remote = withContext(Dispatchers.IO) { api.getDraft(sessionId) } ?: return@launch
                serverDrafts[sessionId] = remote
                val local = chats.get(sessionId)
                if (local.running || remote.updated_at <= (local.draftUpdatedAt ?: 0L)) return@launch
                chats.update(sessionId) {
                    it.copy(
                        input = remote.text,
                        draftUpdatedAt = if (remote.text.trim().isNotEmpty()) remote.updated_at else null,
                    )
                }
            } catch (_: Exception) {
                // Offline: the local draft stays authoritative until the next push.
            }
        }
    }

    private fun scheduleDraftPush(sessionId: String) {
        if (com.console.mobile.core.chat.isNewChatId(sessionId)) return
        draftPushJobs.remove(sessionId)?.cancel()
        draftPushJobs[sessionId] = scope.launch {
            kotlinx.coroutines.delay(DRAFT_PUSH_DEBOUNCE_MS)
            pushDraft(sessionId)
        }
    }

    /** Push immediately, e.g. when leaving the chat; also used after a send clears the composer. */
    fun flushDraft(sessionId: String) {
        if (com.console.mobile.core.chat.isNewChatId(sessionId)) return
        draftPushJobs.remove(sessionId)?.cancel()
        scope.launch { pushDraft(sessionId) }
    }

    private suspend fun pushDraft(sessionId: String) {
        val state = chats.get(sessionId)
        val text = state.input
        val held = serverDrafts[sessionId]
        val draft = console.v1.ChatDraft(
            session_id = sessionId,
            text = text,
            // Chips keep their ranges only while the text they point into is unchanged.
            mentions = if (held?.text == text) held.mentions else emptyList(),
            comments = held?.comments ?: emptyList(),
            // Stamp when the text was last edited, not when the push happens, so a
            // push delayed by being offline cannot overwrite a newer edit elsewhere.
            updated_at = state.draftUpdatedAt ?: System.currentTimeMillis(),
        )
        try {
            val result = withContext(Dispatchers.IO) { api.putDraft(draft) }
            val stored = result.draft
            if (stored != null) serverDrafts[sessionId] = stored
            // Another device wrote later: adopt its text rather than overwrite it next time.
            if (!result.applied && stored != null && chats.get(sessionId).input == text) {
                chats.update(sessionId) {
                    it.copy(
                        input = stored.text,
                        draftUpdatedAt = if (stored.text.trim().isNotEmpty()) stored.updated_at else null,
                    )
                }
            }
        } catch (_: Exception) {
            // Offline: retried on the next edit or when the chat is reopened.
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

    fun loadMessages(sessionId: String, messages: List<AgentMessage>) {
        if (messages.isEmpty()) return
        val withIds = ensureMessageIds(messages)
        chats.update(sessionId) { current ->
            // The server's history is the source of truth even mid-run; the live
            // stream only owns the in-flight buffers and the newest run's
            // activity, which is re-paired with the latest prompt here.
            if (current.running) {
                val merged = mergeLiveHistory(withIds, current.messages)
                current.copy(messages = merged, runs = withLiveRun(merged, current.runs))
            } else {
                current.copy(
                    messages = withIds,
                    streamingText = "",
                    streamingThinking = "",
                    activeToolCalls = emptyList(),
                    pendingQuestions = emptyList(),
                    runs = reconstructRuns(withIds),
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

    /**
     * Prepend an older page. Runs are rebuilt from the whole list because
     * [reconstructRuns] walks messages in order, so runs assembled from a
     * partial tail would misattribute older tool calls. Existing message ids
     * are already populated, so [ensureMessageIds] leaves them untouched and
     * the LazyColumn keys stay stable across the prepend.
     */
    fun prependMessages(sessionId: String, older: List<AgentMessage>) {
        if (older.isEmpty()) return
        chats.update(sessionId) { prependOlderMessages(it, older) }
    }

    fun handleEvent(sessionId: String, event: AgentSessionEvent) {
        if (event.type == "askQuestion") {
            sessions.setStatus(sessionId, SessionStatus.NeedsAttention)
        }
        when (event.type) {
            "askQuestion" -> {
                val req = parseQuestion(event) ?: return
                chats.update(sessionId) {
                    it.copy(pendingQuestions = it.pendingQuestions + PendingQuestion(req))
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
                current.copy(
                    messages = current.messages + AssistantMessage(
                        id = newMessageId(),
                        createdAt = System.currentTimeMillis(),
                        content = listOf(TextPart("Error: The selected model '${view.sessionModelId}' does not support image attachments.")),
                    ),
                )
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
                runs = it.runs + com.console.mobile.core.chat.RunActivityState(
                    runId = newMessageId(),
                    startedAt = System.currentTimeMillis(),
                    status = com.console.mobile.core.chat.RunStatus.Working,
                ),
                attachments = if (clearComposer) emptyList() else it.attachments,
            )
        }
        if (clearComposer) {
            // The prompt no longer lives in the composer; drop the cached
            // draft before the suppressed save window.
            persistence?.removeSession(sessionId)
            // Clear the synced copy too, so other devices stop offering sent text.
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
        persistence?.setSuppress(true)
        try {
            controller.startRun(streamClient)
        } catch (e: Exception) {
            val msg = e.message ?: "Failed to send message. Is the backend running?"
            pendingRuns.remove(sessionId)
            restoreFailedSend(sessionId)
            if (!isAbortError(msg)) markError(sessionId, msg)
            finalize(sessionId, !isAbortError(msg))
            persistence?.setSuppress(false)
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
            persistence?.setSuppress(false)
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
            // The staged prompt lives server-side now; drop the local draft copy
            // and the synced one so other devices stop offering the queued text.
            persistence?.removeSession(sessionId)
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
        chats.update(sessionId) {
            it.copy(running = true, runs = withLiveRun(it.messages, it.runs))
        }
        sessions.setStatus(sessionId, SessionStatus.Working)
        persistence?.setSuppress(true)
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
     * locally, including the unsent draft. The draft is what keeps a deleted
     * chat in the list — the grouping builds a "Drafts" row from any local draft
     * even when the server has no such session.
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
        chats.update(sessionId) { it.copy(messages = it.messages + pending.message) }
    }

    private fun markError(sessionId: String, msg: String) {
        chats.update(sessionId) { s ->
            s.copy(
                messages = s.messages + com.console.mobile.data.model.AssistantMessage(
                    id = newMessageId(),
                    createdAt = System.currentTimeMillis(),
                    content = listOf(com.console.mobile.data.model.TextPart("Error: $msg")),
                ),
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
        persistence?.setSuppress(false)
    }

    // Interactive request payloads are console.v1 (args as JSON bytes),
    // decoded with Moshi; the hand types stay as the UI render models.
    private val wireMoshi: Moshi = Moshi.Builder().add(WireJsonAdapterFactory()).build()
    private val askAdapter = wireMoshi.adapter(console.v1.AskQuestionRequest::class.java)

    private fun parseQuestion(event: AgentSessionEvent): AskQuestionRequest? {
        val raw = event.request?.toString() ?: return null
        val req = runCatching { askAdapter.fromJson(raw) }.getOrNull() ?: return null
        if (req.request_id.isBlank() || req.question.isBlank()) return null
        return AskQuestionRequest(
            requestId = req.request_id,
            question = req.question,
            options = req.options,
            isMultiSelect = req.is_multi_select,
            // The server always sets skippable, but the tool input defaults
            // it to true, so default to true when the key is missing.
            skippable = req.skippable ?: true,
            batchId = req.batch_id,
        )
    }

    private companion object {
        const val DRAFT_PUSH_DEBOUNCE_MS = 800L
    }
}
