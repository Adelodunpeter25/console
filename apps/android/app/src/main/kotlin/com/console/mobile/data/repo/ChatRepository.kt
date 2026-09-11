package com.console.mobile.data.repo

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.PendingPermission
import com.console.mobile.core.chat.PendingQuestion
import com.console.mobile.core.chat.abortSessionRun
import com.console.mobile.core.chat.applyChatEvent
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.ensureMessageIds
import com.console.mobile.core.chat.finalizeSessionRun
import com.console.mobile.core.chat.isAbortError
import com.console.mobile.core.chat.newMessageId
import com.console.mobile.core.chat.toChatSnapshot
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.local.ChatPersistence
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.AgentSessionEvent
import com.console.mobile.data.model.AskQuestionRequest
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.PermissionRequest
import com.console.mobile.data.model.RunPromptDto
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UserMessage
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.stream.ChatStreamClient
import com.console.mobile.data.stream.RunStreamController
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

class ChatRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val streamClient: ChatStreamClient,
    private val chats: ChatStateHolder,
    private val sessions: SessionStateHolder,
    private val persistence: ChatPersistence? = null,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    private val controllers = mutableMapOf<String, RunStreamController>()

    init {
        persistence?.let { p ->
            val cached = p.load()
            if (cached.isNotEmpty()) chats.setAll(cached)
            scope.launch {
                chats.sessions.collect { current ->
                    p.scheduleSave(current)
                }
            }
        }
    }

    fun snapshot(sessionId: String) = toChatSnapshot(chats.get(sessionId))

    fun setInput(sessionId: String, value: String) = chats.setInput(sessionId, value)

    fun loadMessages(sessionId: String, messages: List<AgentMessage>) {
        val current = chats.get(sessionId)
        if (current.running) return
        val withIds = ensureMessageIds(messages)
        chats.update(sessionId) {
            current.copy(
                messages = withIds,
                streamingText = "",
                streamingThinking = "",
                activeToolCalls = emptyList(),
                pendingQuestions = emptyList(),
                pendingPermissions = emptyList(),
                runs = reconstructRuns(withIds),
            )
        }
    }

    fun handleEvent(sessionId: String, event: AgentSessionEvent) {
        if (event.type == "askQuestion" || event.type == "permissionRequest") {
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
            "permissionRequest" -> {
                val req = parsePermission(event) ?: return
                chats.update(sessionId) {
                    it.copy(pendingPermissions = it.pendingPermissions + PendingPermission(req))
                }
                return
            }
            else -> chats.update(sessionId) { applyChatEvent(it, event) }
        }
    }

    /** Send a prompt — appends the user bubble then opens POST /run SSE. */
    fun sendMessage(sessionId: String, promptOverride: String? = null) {
        val session = chats.get(sessionId)
        val prompt = (promptOverride ?: session.input).trim()
        if (prompt.isEmpty() || session.running) return
        val view = sessions.getView(sessionId)
        val attachments = session.attachments

        chats.update(sessionId) {
            it.copy(
                messages = it.messages + UserMessage(
                    id = newMessageId(),
                    createdAt = System.currentTimeMillis(),
                    content = prompt,
                    attachments = emptyList(),
                ),
                input = "",
                streamingText = "",
                streamingThinking = "",
                activeToolCalls = emptyList(),
                running = true,
                runs = it.runs + com.console.mobile.core.chat.RunActivityState(
                    runId = newMessageId(),
                    startedAt = System.currentTimeMillis(),
                    status = com.console.mobile.core.chat.RunStatus.Working,
                ),
                attachments = emptyList(),
            )
        }
        sessions.setStatus(sessionId, SessionStatus.Working)

        val body = RunPromptDto(
            prompt = prompt,
            modelId = view.sessionModelId,
            provider = view.sessionProvider,
            approvalMode = view.approvalMode.takeIf { it.isNotBlank() },
            attachments = attachments,
        )
        val bodyJson = ConsoleJson.encodeToString(RunPromptDto.serializer(), body)
        val controller = getOrCreate(sessionId, bodyJson)
        persistence?.setSuppress(true)
        try {
            controller.startRun(streamClient)
        } catch (e: Exception) {
            val msg = e.message ?: "Failed to send message. Is the backend running?"
            if (!isAbortError(msg)) markError(sessionId, msg)
            finalize(sessionId, !isAbortError(msg))
            persistence?.setSuppress(false)
        }
    }

    /** User stop — kill stream/timers, abort server run, mark aborted. */
    fun abort(sessionId: String) {
        controllers[sessionId]?.cancel()
        scope.launch {
            try {
                withContext(Dispatchers.IO) { api.abortRun(sessionId) }
            } catch (_: Exception) {
            }
            chats.update(sessionId) { abortSessionRun(it) }
            sessions.setStatus(sessionId, SessionStatus.Done)
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
            val runs = it.runs
            val hasWorking = runs.isNotEmpty() && runs.last().status == com.console.mobile.core.chat.RunStatus.Working
            it.copy(
                running = true,
                runs = if (hasWorking) runs else runs + com.console.mobile.core.chat.RunActivityState(
                    runId = newMessageId(),
                    startedAt = System.currentTimeMillis(),
                    status = com.console.mobile.core.chat.RunStatus.Working,
                ),
            )
        }
        sessions.setStatus(sessionId, SessionStatus.Working)
        persistence?.setSuppress(true)
        try {
            // since=0 replays the whole current run buffer.
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

    fun approvePermission(sessionId: String, requestId: String, allow: Boolean) {
        scope.launch {
            try {
                withContext(Dispatchers.IO) {
                    api.approvePermission(sessionId, com.console.mobile.data.model.ApproveToolPermissionDto(requestId, allow))
                }
            } catch (_: Exception) {
            }
            chats.update(sessionId) { s ->
                s.copy(pendingPermissions = s.pendingPermissions.filterNot { it.request.requestId == requestId })
            }
        }
    }

    fun clear(sessionId: String) = chats.clear(sessionId)

    private fun getOrCreate(sessionId: String, bodyJson: String): RunStreamController {
        return controllers.getOrPut(sessionId) {
            RunStreamController(sessionId, object : RunStreamController.Deps {
                override fun handleEvent(event: AgentSessionEvent) = this@ChatRepository.handleEvent(sessionId, event)
                override fun markError(message: String) = markError(sessionId, message)
                override fun finalize(hadError: Boolean) {
                    this@ChatRepository.finalize(sessionId, hadError)
                    controllers.remove(sessionId)
                }
                override fun baseUrl(): String = apiClient.baseUrl
                override fun authToken(): String? = apiClient.authToken
                override fun runBodyJson(): String = bodyJson
                override fun scope(): CoroutineScope = this@ChatRepository.scope
            })
        }
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
        chats.update(sessionId) { finalizeSessionRun(it, hadError) }
        sessions.setStatus(sessionId, if (hadError) SessionStatus.NeedsAttention else SessionStatus.Done)
        persistence?.setSuppress(false)
    }

    private fun parseQuestion(event: AgentSessionEvent): AskQuestionRequest? {
        val req = event.request as? JsonObject ?: return null
        fun str(key: String): String? = (req[key] as? kotlinx.serialization.json.JsonPrimitive)?.contentOrNull
        val requestId = str("requestId") ?: return null
        val question = str("question") ?: return null
        return AskQuestionRequest(
            requestId = requestId,
            question = question,
            options = emptyList(),
            isMultiSelect = false,
            skippable = false,
            batchId = str("batchId"),
        )
    }

    private fun parsePermission(event: AgentSessionEvent): PermissionRequest? {
        val req = event.request as? JsonObject ?: return null
        fun str(key: String): String? = (req[key] as? JsonPrimitive)?.contentOrNull
        val requestId = str("requestId") ?: return null
        return PermissionRequest(
            requestId = requestId,
            toolCallId = str("toolCallId").orEmpty(),
            toolName = str("toolName").orEmpty(),
        )
    }
}
