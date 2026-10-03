package com.console.mobile.data.store

import com.console.mobile.data.model.FsTreeEntry
import com.console.mobile.data.model.McpServerEntry
import com.console.mobile.data.model.ProviderAuthStatus
import com.console.mobile.data.model.TerminalSpawnedEvent
import com.console.mobile.data.model.UsageReport
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.serialization.Serializable

data class AuthState(
    val status: Map<String, ProviderAuthStatus>? = null,
    val loading: Boolean = false,
    val loggingIn: String? = null,
    val error: String? = null,
    val projectIds: Map<String, String?> = emptyMap(),
    val savingProjectId: Boolean = false,
)

class AuthStateHolder(initial: AuthState = AuthState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<AuthState> = _state.asStateFlow()
    fun set(v: AuthState) { _state.value = v }
    fun patch(fn: (AuthState) -> AuthState) { _state.value = fn(_state.value) }
}

data class FsState(
    val browsePath: String? = null,
    val browseEntries: List<FsTreeEntry> = emptyList(),
    val browsing: Boolean = false,
    val treesByPath: Map<String, String> = emptyMap(),
    val fileContentsByPath: Map<String, String> = emptyMap(),
    val busyPaths: Set<String> = emptySet(),
    val error: String? = null,
)

class FsStateHolder(initial: FsState = FsState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<FsState> = _state.asStateFlow()
    fun set(v: FsState) { _state.value = v }
    fun patch(fn: (FsState) -> FsState) { _state.value = fn(_state.value) }
    fun markBusy(path: String) { patch { it.copy(busyPaths = it.busyPaths + path) } }
    fun clearBusy(path: String) { patch { it.copy(busyPaths = it.busyPaths - path) } }
    fun invalidateFile(path: String) { patch { it.copy(fileContentsByPath = it.fileContentsByPath - path) } }
}

data class UsageState(
    val reports: Map<String, UsageReport?> = emptyMap(),
    val loading: Boolean = false,
    val loadingByProvider: Map<String, Boolean> = emptyMap(),
    val error: String? = null,
    val lastFetchedAt: Long? = null,
)

class UsageStateHolder(initial: UsageState = UsageState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<UsageState> = _state.asStateFlow()
    fun set(v: UsageState) { _state.value = v }
    fun patch(fn: (UsageState) -> UsageState) { _state.value = fn(_state.value) }
    fun invalidate(providerId: String? = null) {
        patch { s ->
            if (providerId != null) s.copy(reports = s.reports - providerId, loadingByProvider = s.loadingByProvider + (providerId to false))
            else s.copy(reports = emptyMap(), loadingByProvider = emptyMap(), lastFetchedAt = null)
        }
    }
}

data class McpState(
    val servers: List<McpServerEntry> = emptyList(),
    val loading: Boolean = false,
    val busyServerIds: Set<String> = emptySet(),
    val error: String? = null,
)

class McpStateHolder(initial: McpState = McpState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<McpState> = _state.asStateFlow()
    fun set(v: McpState) { _state.value = v }
    fun patch(fn: (McpState) -> McpState) { _state.value = fn(_state.value) }
    fun entry(id: String): McpServerEntry? = _state.value.servers.firstOrNull { it.id == id }
}

enum class TerminalStatus { Spawning, Running, Exited, Error }

data class TerminalRecord(
    val id: String,
    val projectId: String,
    val status: TerminalStatus,
    val pid: Int? = null,
    val shell: String? = null,
    val cwd: String? = null,
    val cols: Int = 80,
    val rows: Int = 24,
    val error: String? = null,
    val revision: Int = 0,
)

class TerminalStateHolder {
    private val _terminals = MutableStateFlow<Map<String, TerminalRecord>>(emptyMap())
    val terminals: StateFlow<Map<String, TerminalRecord>> = _terminals.asStateFlow()
    private val _buffers = MutableStateFlow<Map<String, String>>(emptyMap())
    val buffers: StateFlow<Map<String, String>> = _buffers.asStateFlow()

    // Terminal frames arrive on OkHttp's WebSocket reader thread, so every
    // mutation below must be atomic. A plain read-modify-write on `.value` can
    // read a stale map and CAS it back over a concurrent update from another
    // terminal's reader thread, silently dropping that chunk. `update` retries
    // the transform on contention instead.
    fun ensure(record: TerminalRecord) {
        _terminals.update { if (it.containsKey(record.id)) it else it + (record.id to record) }
    }
    fun set(id: String, record: TerminalRecord) { _terminals.update { it + (id to record) } }
    fun patch(id: String, fn: (TerminalRecord) -> TerminalRecord) {
        _terminals.update { m ->
            val cur = m[id] ?: return@update m
            m + (id to fn(cur).copy(revision = cur.revision + 1))
        }
    }
    fun remove(id: String) { _terminals.update { it - id }; _buffers.update { it - id } }
    fun appendOutput(id: String, data: String) {
        _buffers.update { it + (id to ((it[id] ?: "") + data)) }
    }
    fun findLive(projectId: String, cwd: String?): String? {
        val cands = _terminals.value.values.filter { it.projectId == projectId && (it.status == TerminalStatus.Spawning || it.status == TerminalStatus.Running) }
        if (cwd != null) cands.firstOrNull { it.cwd == cwd }?.let { return it.id }
        return cands.firstOrNull()?.id
    }
    fun clear() { _terminals.value = emptyMap(); _buffers.value = emptyMap() }
}

@Serializable
data class Environment(val id: String, val name: String, val url: String)

data class EnvironmentsState(
    val environments: List<Environment> = emptyList(),
    val activeId: String? = null,
    val probes: Map<String, Boolean> = emptyMap(),
)

fun activeUrlOf(s: EnvironmentsState): String? = s.environments.firstOrNull { it.id == s.activeId }?.url

class EnvironmentsStateHolder(initial: EnvironmentsState = EnvironmentsState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<EnvironmentsState> = _state.asStateFlow()
    fun set(v: EnvironmentsState) { _state.value = v }
    fun patch(fn: (EnvironmentsState) -> EnvironmentsState) { _state.value = fn(_state.value) }
}
