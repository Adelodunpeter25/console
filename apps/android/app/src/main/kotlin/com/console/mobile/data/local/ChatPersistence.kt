package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences
import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.ensureMessageIds
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentMessage
import com.console.mobile.data.model.ImageAttachment
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable

@Serializable
data class PersistedSessionPartial(
    val messages: List<AgentMessage> = emptyList(),
    val runs: List<RunActivityState> = emptyList(),
    val input: String = "",
    val attachments: List<ImageAttachment> = emptyList(),
    val draftUpdatedAt: Long? = null,
)

@Serializable
data class PersistedChatPayload(
    val version: Int = ChatPersistence.PERSIST_VERSION,
    val sessions: Map<String, PersistedSessionPartial> = emptyMap(),
)

class ChatPersistence(
    private val prefs: SharedPreferences,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.IO),
) {
    companion object {
        const val PREFS_NAME = "console_chat_storage"
        const val PERSIST_KEY = "console-chat-cache"
        const val PERSIST_VERSION = 1
        const val MAX_PERSISTED_SESSIONS = 25
        const val MAX_PERSISTED_MESSAGES = 50
        const val SAVE_THROTTLE_MS = 300L

        fun create(context: Context): ChatPersistence {
            val p = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            return ChatPersistence(p)
        }
    }

    private var suppress = false
    private var savePendingWhileSuppressed = false
    private var saveJob: Job? = null
    private var lastObservedSessions: Map<String, ChatSessionState> = emptyMap()

    fun setSuppress(value: Boolean) {
        suppress = value
        if (!value && savePendingWhileSuppressed) {
            savePendingWhileSuppressed = false
            saveNow(lastObservedSessions)
        }
    }

    fun scheduleSave(sessions: Map<String, ChatSessionState>) {
        lastObservedSessions = sessions
        if (suppress) {
            savePendingWhileSuppressed = true
            return
        }
        if (saveJob?.isActive == true) return
        saveJob = scope.launch {
            delay(SAVE_THROTTLE_MS)
            saveNow(lastObservedSessions)
        }
    }

    fun saveNow(sessions: Map<String, ChatSessionState>) {
        try {
            val capped = buildPersistedPayload(sessions)
            val json = ConsoleJson.encodeToString(PersistedChatPayload.serializer(), capped)
            prefs.edit().putString(PERSIST_KEY, json).apply()
        } catch (_: Exception) {
        }
    }

    fun load(): Map<String, ChatSessionState> {
        val raw = prefs.getString(PERSIST_KEY, null) ?: return emptyMap()
        return try {
            val payload = ConsoleJson.decodeFromString(PersistedChatPayload.serializer(), raw)
            payload.sessions.mapValues { (_, partial) ->
                val withIds = ensureMessageIds(partial.messages)
                createChatSessionState().copy(
                    messages = withIds,
                    runs = partial.runs,
                    input = partial.input,
                    attachments = partial.attachments.take(2),
                    draftUpdatedAt = partial.draftUpdatedAt,
                )
            }
        } catch (_: Exception) {
            emptyMap()
        }
    }

    fun clear() {
        prefs.edit().remove(PERSIST_KEY).apply()
    }

    private fun hasPersistableDraft(s: ChatSessionState): Boolean =
        s.input.trim().isNotEmpty() || s.attachments.isNotEmpty()

    private fun buildPersistedPayload(sessions: Map<String, ChatSessionState>): PersistedChatPayload {
        val filtered = sessions.entries
            .filter { (_, s) -> s.messages.isNotEmpty() || hasPersistableDraft(s) }
            .sortedWith { a, b ->
                val aDraft = if (hasPersistableDraft(a.value)) a.value.draftUpdatedAt ?: 0L else 0L
                val bDraft = if (hasPersistableDraft(b.value)) b.value.draftUpdatedAt ?: 0L else 0L
                if (aDraft != bDraft) bDraft.compareTo(aDraft)
                else b.value.messages.size.compareTo(a.value.messages.size)
            }
            .take(MAX_PERSISTED_SESSIONS)
            .associate { (id, s) ->
                id to PersistedSessionPartial(
                    messages = s.messages.takeLast(MAX_PERSISTED_MESSAGES),
                    runs = s.runs,
                    input = s.input,
                    attachments = s.attachments.take(2),
                    draftUpdatedAt = s.draftUpdatedAt,
                )
            }
        return PersistedChatPayload(version = PERSIST_VERSION, sessions = filtered)
    }
}
