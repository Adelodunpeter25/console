package com.console.mobile.core.chat

import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.BackgroundWork
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.AskQuestionRequest
import console.v1.ContextSnapshot
import console.v1.SubagentInfo
import console.v1.TodoItem
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.Serializable

data class PendingQuestion(val request: AskQuestionRequest)

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
    // Set on a server wake turn (backgroundWorkContinue): the run renders
    // under the same user message as the run before it, introduced by this
    // divider label instead of a new prompt bubble.
    val dividerLabel: String? = null,
)

/**
 * One recorded wake turn for a session: when a `backgroundWorkContinue`
 * frame arrived and what it said. Mirrors the desktop WakeRecord; kept so
 * run boundaries can be rebuilt after history reloads.
 */
@Serializable
data class WakeRecord(
    val message: String,
    /** Client receipt time, epoch millis. */
    val at: Long,
)

data class ChatSessionState(
    val messages: List<AgentMessage> = emptyList(),
    val input: String = "",
    val running: Boolean = false,
    val streamingText: String = "",
    val streamingThinking: String = "",
    val pendingQuestions: List<PendingQuestion> = emptyList(),
    val activeToolCalls: List<ToolCall> = emptyList(),
    val todoItems: List<TodoItem> = emptyList(),
    val subagents: List<SubagentInfo> = emptyList(),
    val backgroundWork: List<BackgroundWork> = emptyList(),
    val runs: List<RunActivityState> = emptyList(),
    val queuedPrompt: console.v1.QueuedPrompt? = null,
    // Follow-up prompt chips; the server keeps the latest set, so this is a
    // cache that is never persisted on the phone.
    val suggestions: List<String> = emptyList(),
    // Context-window occupancy for the footer ring; null until first known.
    val context: ContextSnapshot? = null,
    val attachments: List<ImageAttachment> = emptyList(),
    val draftUpdatedAt: Long? = null,
    // Bumped by actions that put text in the composer (e.g. Edit Before Sending)
    // so it takes focus even when the tap happened outside it.
    val focusRequest: Int = 0,
    // Message pagination. The session API returns the newest page on open;
    // `nextCursor` is the rowid to pass back as `before` to walk backwards.
    val hasMoreMessages: Boolean = false,
    val nextCursor: Long? = null,
    val loadingOlder: Boolean = false,
    // Recorded backgroundWorkContinue frames; drives wake-run splitting in
    // reconstructRuns after history reloads. Mirrors desktop wake_records.
    val wakeRecords: List<WakeRecord> = emptyList(),
)

val EMPTY_CHAT_SESSION = ChatSessionState()

data class ChatSnapshot(
    val messages: List<AgentMessage> = emptyList(),
    val streamingText: String = "",
    val streamingThinking: String = "",
    val activeToolCalls: List<ToolCall> = emptyList(),
    val liveToolResults: List<ToolResult> = emptyList(),
    val pendingQuestion: PendingQuestion? = null,
    val pendingQuestions: List<PendingQuestion> = emptyList(),
    val todoItems: List<TodoItem> = emptyList(),
    val subagents: List<SubagentInfo> = emptyList(),
    val backgroundWork: List<BackgroundWork> = emptyList(),
    val running: Boolean = false,
    val runs: List<RunActivityState> = emptyList(),
    val queuedPrompt: console.v1.QueuedPrompt? = null,
)

fun createChatSessionState(): ChatSessionState = ChatSessionState()

/** Only detached shells that are still running: what the chat chip and sheet show. */
fun runningBackgroundShells(works: List<BackgroundWork>): List<BackgroundWork> =
    works.filter { it.kind == "shell" && it.status == "running" }
