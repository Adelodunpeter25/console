package com.console.mobile.data.store

import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.createChatSessionState
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** Port of useChatStore.ts session map (streaming coalescing simplified: direct apply). */
class ChatStateHolder {
    private val _sessions = MutableStateFlow<Map<String, ChatSessionState>>(emptyMap())
    val sessions: StateFlow<Map<String, ChatSessionState>> = _sessions.asStateFlow()

    fun get(id: String): ChatSessionState = _sessions.value[id] ?: createChatSessionState()

    fun update(id: String, fn: (ChatSessionState) -> ChatSessionState) {
        _sessions.value = _sessions.value + (id to fn(get(id)))
    }

    fun setInput(id: String, value: String) = update(id) { it.copy(input = value) }
    fun clear(id: String) { _sessions.value = _sessions.value + (id to createChatSessionState()) }
}
