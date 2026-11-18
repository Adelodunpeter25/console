package com.console.mobile.data.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

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
    /** When true, open an HTML file directly in the code editor instead of prompting. */
    val pendingFileForceEditor: Boolean = false,
)

class AppStateHolder(initial: AppState = AppState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<AppState> = _state.asStateFlow()

    fun setActiveTab(tab: MobileTab) {
        _state.update { cur ->
            if (cur.activeTab == tab) cur
            else cur.copy(previousTab = cur.activeTab, activeTab = tab)
        }
    }

    /** Jump to the Files tab and open [path] there. */
    fun openFile(path: String, forceEditor: Boolean = false) {
        _state.update { cur ->
            cur.copy(
                previousTab = if (cur.activeTab != MobileTab.Files) cur.activeTab else cur.previousTab,
                activeTab = MobileTab.Files,
                pendingFilePath = path,
                pendingFileForceEditor = forceEditor,
            )
        }
    }

    fun consumePendingFile() { _state.update { it.copy(pendingFilePath = null, pendingFileForceEditor = false) } }

    fun setSelectedProjectId(id: String?) { _state.update { it.copy(selectedProjectId = id) } }
    fun setSelectedSessionId(id: String?) { _state.update { it.copy(selectedSessionId = id) } }
    fun setBackendUrl(url: String?) { _state.update { it.copy(backendUrl = url) } }
    fun setPendingServersSection(pending: Boolean) { _state.update { it.copy(pendingServersSection = pending) } }

    fun openChatSession(sessionId: String) {
        _state.update { cur ->
            cur.copy(
                previousTab = if (cur.activeTab != MobileTab.Chat) cur.activeTab else cur.previousTab,
                activeTab = MobileTab.Chat,
                selectedSessionId = sessionId,
            )
        }
    }

    fun clearSelections() {
        _state.update { cur ->
            cur.copy(
                activeTab = if (cur.activeTab == MobileTab.Chat) MobileTab.Home else cur.activeTab,
                selectedProjectId = null,
                selectedSessionId = null,
            )
        }
    }
}
