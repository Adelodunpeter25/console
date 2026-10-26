package com.console.mobile.data.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

enum class MobileTab { Home, Chat, Settings, Terminal, Files, Changes, Devices }

data class AppState(
    val activeTab: MobileTab = MobileTab.Home,
    val previousTab: MobileTab? = null,
    val selectedProjectId: String? = null,
    val selectedSessionId: String? = null,
    val backendUrl: String? = null,
    val pendingServersSection: Boolean = false,
    /** A file a link asked to open; the Files screen consumes it. */
    val pendingFilePath: String? = null,
)

class AppStateHolder(initial: AppState = AppState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<AppState> = _state.asStateFlow()

    fun setActiveTab(tab: MobileTab) {
        val cur = _state.value
        if (cur.activeTab == tab) return
        _state.value = cur.copy(previousTab = cur.activeTab, activeTab = tab)
    }

    /** Jump to the Files tab and open [path] there. */
    fun openFile(path: String) {
        val cur = _state.value
        _state.value = cur.copy(
            previousTab = if (cur.activeTab != MobileTab.Files) cur.activeTab else cur.previousTab,
            activeTab = MobileTab.Files,
            pendingFilePath = path,
        )
    }

    fun consumePendingFile() { _state.value = _state.value.copy(pendingFilePath = null) }

    fun setSelectedProjectId(id: String?) { _state.value = _state.value.copy(selectedProjectId = id) }
    fun setSelectedSessionId(id: String?) { _state.value = _state.value.copy(selectedSessionId = id) }
    fun setBackendUrl(url: String?) { _state.value = _state.value.copy(backendUrl = url) }
    fun setPendingServersSection(pending: Boolean) { _state.value = _state.value.copy(pendingServersSection = pending) }

    fun openChatSession(sessionId: String) {
        val cur = _state.value
        _state.value = cur.copy(
            previousTab = if (cur.activeTab != MobileTab.Chat) cur.activeTab else cur.previousTab,
            activeTab = MobileTab.Chat,
            selectedSessionId = sessionId,
        )
    }

    fun clearSelections() {
        val cur = _state.value
        _state.value = cur.copy(
            activeTab = if (cur.activeTab == MobileTab.Chat) MobileTab.Home else cur.activeTab,
            selectedProjectId = null,
            selectedSessionId = null,
        )
    }
}
