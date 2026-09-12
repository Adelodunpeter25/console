package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.PermissionRequest
import com.console.mobile.data.model.AskQuestionRequest
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.Serializable

data class PendingQuestion(val request: AskQuestionRequest)
data class PendingPermission(val request: PermissionRequest)

enum class RunStatus { Working, Completed, Aborted, Failed }

@Serializable
sealed interface ActivityEvent {
    val id: String
    @Serializable
    data class Text(override val id: String, val text: String) : ActivityEvent
    @Serializable
    data class Thinking(override val id: String, val text: String) : ActivityEvent
    @Serializable
    data class ToolCallEvent(override val id: String, val call: ToolCall, val result: ToolResult? = null) : ActivityEvent
}

@Serializable
data class RunActivityState(
    val runId: String,
    val startedAt: Long? = null,
    val elapsedMs: Long = 0,
    val events: List<ActivityEvent> = emptyList(),
    val status: RunStatus = RunStatus.Completed,
)

data class ChatSessionState(
    val messages: List<AgentMessage> = emptyList(),
    val input: String = "",
    val running: Boolean = false,
    val streamingText: String = "",
    val streamingThinking: String = "",
    val pendingQuestions: List<PendingQuestion> = emptyList(),
    val pendingPermissions: List<PendingPermission> = emptyList(),
    val activeToolCalls: List<ToolCall> = emptyList(),
    val todoItems: List<TodoItem> = emptyList(),
    val subagents: List<SubagentInfo> = emptyList(),
    val runs: List<RunActivityState> = emptyList(),
    val attachments: List<ImageAttachment> = emptyList(),
    val draftUpdatedAt: Long? = null,
)

val EMPTY_CHAT_SESSION = ChatSessionState()

data class ChatSnapshot(
    val messages: List<AgentMessage> = emptyList(),
    val streamingText: String = "",
    val streamingThinking: String = "",
    val activeToolCalls: List<ToolCall> = emptyList(),
    val liveToolResults: List<ToolResult> = emptyList(),
    val pendingPermission: PendingPermission? = null,
    val pendingQuestion: PendingQuestion? = null,
    val pendingPermissions: List<PendingPermission> = emptyList(),
    val pendingQuestions: List<PendingQuestion> = emptyList(),
    val todoItems: List<TodoItem> = emptyList(),
    val subagents: List<SubagentInfo> = emptyList(),
    val running: Boolean = false,
    val runs: List<RunActivityState> = emptyList(),
)

fun createChatSessionState(): ChatSessionState = ChatSessionState()
