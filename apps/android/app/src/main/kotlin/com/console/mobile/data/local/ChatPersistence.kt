package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences
import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.ImageAttachment
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable

/**
 * Only the unsent draft is kept on disk. Messages and run activity always come
 * from the server: a cached transcript carried client-made message ids that
 * never matched the server's, and stale finished runs, so a chat reopened
 * mid-run lost its live tool calls under duplicated or misaligned prompts.
 * Older payloads that still hold `messages`/`runs` decode fine — unknown keys
 * are ignored.
 */
@Serializable
data class PersistedSessionPartial(
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
        /**
         * Draft images kept in the on-disk cache. This is a storage bound, not
         * a UX limit — the composer accepts any number (see AttachmentStrip,
         * which paginates). The cache re-encodes every attachment to base64
         * inside one JSON blob written to SharedPreferences on a 300ms
         * throttle, so the count has to stay small or each save rewrites
         * megabytes. A draft larger than this loses the overflow on restore.
         */
        const val MAX_PERSISTED_ATTACHMENTS = 3
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
            payload.sessions.filterValues { hasDraft(it.input, it.attachments) }.mapValues { (_, partial) ->
                createChatSessionState().copy(
                    input = partial.input,
                    attachments = partial.attachments.take(MAX_PERSISTED_ATTACHMENTS),
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

    private fun hasDraft(input: String, attachments: List<ImageAttachment>): Boolean =
        input.trim().isNotEmpty() || attachments.isNotEmpty()

    private fun buildPersistedPayload(sessions: Map<String, ChatSessionState>): PersistedChatPayload {
        val filtered = sessions.entries
            .filter { (_, s) -> hasDraft(s.input, s.attachments) }
            .sortedByDescending { (_, s) -> s.draftUpdatedAt ?: 0L }
            .take(MAX_PERSISTED_SESSIONS)
            .associate { (id, s) ->
                id to PersistedSessionPartial(
                    input = s.input,
                    attachments = s.attachments.take(MAX_PERSISTED_ATTACHMENTS),
                    draftUpdatedAt = s.draftUpdatedAt,
                )
            }
        return PersistedChatPayload(version = PERSIST_VERSION, sessions = filtered)
    }
}
