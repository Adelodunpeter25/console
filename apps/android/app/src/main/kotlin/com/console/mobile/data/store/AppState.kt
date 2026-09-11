package com.console.mobile.data.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

enum class MobileTab { Home, Chat, Settings, Terminal, Files, Changes, Subagents, SubagentDetails }

data class AppState(
    val activeTab: MobileTab = MobileTab.Home,
    val previousTab: MobileTab? = null,
    val selectedProjectId: String? = null,
    val selectedSessionId: String? = null,
    val selectedSubagentId: String? = null,
    val backendUrl: String? = null,
    val pendingConnectionSection: Boolean = false,
)

class AppStateHolder(initial: AppState = AppState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<AppState> = _state.asStateFlow()

    fun setActiveTab(tab: MobileTab) {
        val cur = _state.value
        if (cur.activeTab == tab) return
        _state.value = cur.copy(previousTab = cur.activeTab, activeTab = tab)
    }

    fun setSelectedProjectId(id: String?) { _state.value = _state.value.copy(selectedProjectId = id) }
    fun setSelectedSessionId(id: String?) { _state.value = _state.value.copy(selectedSessionId = id) }
    fun setSelectedSubagentId(id: String?) { _state.value = _state.value.copy(selectedSubagentId = id) }
    fun setBackendUrl(url: String?) { _state.value = _state.value.copy(backendUrl = url) }
    fun setPendingConnectionSection(pending: Boolean) { _state.value = _state.value.copy(pendingConnectionSection = pending) }

    fun openChatSession(sessionId: String) {
        val cur = _state.value
        _state.value = cur.copy(
            previousTab = if (cur.activeTab != MobileTab.Chat) cur.activeTab else cur.previousTab,
            activeTab = MobileTab.Chat,
            selectedSessionId = sessionId,
        )
    }

    fun clearSelections() {
        _state.value = _state.value.copy(selectedProjectId = null, selectedSessionId = null, selectedSubagentId = null)
    }
}
