package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.McpOAuthCallbackPayload
import com.console.mobile.data.model.McpSavePayload
import com.console.mobile.data.model.McpServerEntry
import com.console.mobile.data.store.McpStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * MCP servers: list / save / delete / connect / disconnect plus the OAuth
 * callback forward. Mirrors the desktop settings flow — connect starts the
 * server-side flow, the UI opens `authUrl` on `needs_auth`, forwards the
 * captured result, then polls to a terminal state.
 */
class McpRepository(
    private val api: ConsoleApi,
    private val mcpState: McpStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    companion object {
        /** Poll cadence mirrors the desktop client's 250ms. */
        const val POLL_MS = 250L
        /** Upper bound waiting for discovery + registration before auth. */
        const val SETTLE_TIMEOUT_MS = 60_000L
    }

    fun loadServers() {
        scope.launch {
            mcpState.patch { it.copy(loading = true, error = null) }
            try {
                val servers = withContext(Dispatchers.IO) { api.listMcpServers() }
                mcpState.patch { it.copy(servers = servers, loading = false, error = null) }
            } catch (e: Exception) {
                mcpState.patch { it.copy(loading = false, error = e.message ?: "Failed to load MCP servers") }
            }
        }
    }

    fun saveServer(payload: McpSavePayload, id: String? = null, onDone: (Boolean) -> Unit = {}) {
        scope.launch {
            mcpState.patch { it.copy(error = null) }
            setBusy(id, true)
            try {
                withContext(Dispatchers.IO) {
                    if (id != null) api.updateMcpServer(id, payload) else api.saveMcpServer(payload)
                }
                refresh()
                onDone(true)
            } catch (e: Exception) {
                mcpState.patch { it.copy(error = e.message ?: "Failed to save MCP server") }
                onDone(false)
            } finally {
                setBusy(id, false)
            }
        }
    }

    fun deleteServer(id: String) {
        scope.launch {
            setBusy(id, true)
            try {
                withContext(Dispatchers.IO) { api.deleteMcpServer(id) }
                refresh()
            } catch (e: Exception) {
                mcpState.patch { it.copy(error = e.message ?: "Failed to delete MCP server") }
            } finally {
                setBusy(id, false)
            }
        }
    }

    fun disconnectServer(id: String) {
        scope.launch {
            setBusy(id, true)
            try {
                withContext(Dispatchers.IO) { api.disconnectMcpServer(id) }
                refresh()
            } catch (e: Exception) {
                mcpState.patch { it.copy(error = e.message ?: "Failed to disconnect MCP server") }
            } finally {
                setBusy(id, false)
            }
        }
    }

    /**
     * Starts the server-side connect with an optional client loopback
     * redirect, then polls until the row leaves `connecting`. Returns the
     * settled entry (or null when the row vanished / the call failed). The
     * caller opens `authUrl` on `needs_auth` and forwards the captured
     * result via [forwardOAuthCallback], then calls [awaitTerminal].
     */
    suspend fun startConnect(id: String, redirectUri: String?): McpServerEntry? {
        return try {
            withContext(Dispatchers.IO) { api.connectMcpServer(id, redirectUri) }
            pollUntil(id, SETTLE_TIMEOUT_MS) { it.status != "connecting" }
        } catch (e: Exception) {
            mcpState.patch { it.copy(error = e.message ?: "Failed to connect MCP server") }
            null
        }
    }

    /** Polls until the row reaches a terminal state (connected / error / disconnected). */
    suspend fun awaitTerminal(id: String, timeoutMs: Long = 90_000L): McpServerEntry? =
        pollUntil(id, timeoutMs) { it.status != "connecting" && it.status != "needs_auth" }

    suspend fun forwardOAuthCallback(id: String, state: String, code: String?, error: String?): Boolean {
        return try {
            withContext(Dispatchers.IO) {
                api.forwardMcpOAuthCallback(id, McpOAuthCallbackPayload(state = state, code = code, error = error))
            }
            true
        } catch (e: Exception) {
            mcpState.patch { it.copy(error = e.message ?: "Failed to complete browser sign-in") }
            false
        }
    }

    private suspend fun refresh() {
        try {
            val servers = withContext(Dispatchers.IO) { api.listMcpServers() }
            mcpState.patch { it.copy(servers = servers, error = null) }
        } catch (e: Exception) {
            mcpState.patch { it.copy(error = e.message ?: "Failed to load MCP servers") }
        }
    }

    private suspend fun pollUntil(id: String, timeoutMs: Long, done: (McpServerEntry) -> Boolean): McpServerEntry? {
        val deadline = System.currentTimeMillis() + timeoutMs
        var last: McpServerEntry? = null
        while (System.currentTimeMillis() < deadline) {
            try {
                val servers = withContext(Dispatchers.IO) { api.listMcpServers() }
                mcpState.patch { it.copy(servers = servers, error = null) }
                last = servers.firstOrNull { it.id == id }
                if (last != null && done(last)) return last
            } catch (e: Exception) {
                mcpState.patch { it.copy(error = e.message ?: "Failed to load MCP servers") }
                return last
            }
            delay(POLL_MS)
        }
        return last
    }

    private fun setBusy(id: String?, busy: Boolean) {
        if (id == null) return
        mcpState.patch {
            it.copy(busyServerIds = if (busy) it.busyServerIds + id else it.busyServerIds - id)
        }
    }
}
