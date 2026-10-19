package com.console.mobile.data.repo

import com.console.mobile.core.forward.LocalForward
import com.console.mobile.core.forward.LocalForwardManager
import com.console.mobile.data.api.ConsoleApi
import console.v1.ForwardedPort
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext

/**
 * Forwarded dev ports (`/api/ports`) plus the loopback tunnels that make them
 * openable from this device's built-in browser.
 *
 * The `url` the server reports per port is a LAN proxy address (built from the
 * caller's Host header), so it only works while the phone is on the same
 * network as the backend. [openUrl] prefers a `127.0.0.1` tunnel and keeps the
 * server URL as the fallback.
 */
class PortsRepository(
    private val api: ConsoleApi,
    private val forwards: LocalForwardManager,
) {
    private val _ports = MutableStateFlow<List<ForwardedPort>>(emptyList())
    val ports: StateFlow<List<ForwardedPort>> = _ports.asStateFlow()

    /** Loopback URLs for the ports above, once their listeners are up. */
    val locals: StateFlow<List<LocalForward>> = forwards.forwards

    private val _isLoading = MutableStateFlow(false)
    val isLoading: StateFlow<Boolean> = _isLoading.asStateFlow()

    private val _error = MutableStateFlow<String?>(null)
    val error: StateFlow<String?> = _error.asStateFlow()

    /** Refresh the list and reconcile loopback listeners with it. */
    suspend fun refresh(projectId: String? = null): Result<List<ForwardedPort>> =
        withContext(Dispatchers.IO) {
            _isLoading.value = true
            _error.value = null
            try {
                val ports = api.getPorts(projectId)
                _ports.value = ports
                forwards.reconcile(ports.map { it.port })
                _isLoading.value = false
                Result.success(ports)
            } catch (e: Exception) {
                _error.value = e.message ?: "Failed to load ports"
                _isLoading.value = false
                Result.failure(e)
            }
        }

    /** Ask the server to forward [port] and open a loopback listener for it. */
    suspend fun forward(port: Int, projectId: String? = null): Result<ForwardedPort> =
        withContext(Dispatchers.IO) {
            try {
                val forwarded = api.forwardPort(port, projectId)
                _ports.value = (_ports.value.filterNot { it.port == forwarded.port } + forwarded)
                    .sortedBy { it.port }
                forwards.ensure(forwarded.port)
                Result.success(forwarded)
            } catch (e: Exception) {
                _error.value = e.message ?: "Failed to forward port"
                Result.failure(e)
            }
        }

    /** Stop forwarding [port]: server entry plus loopback listener. */
    suspend fun unforward(port: Int, projectId: String? = null): Result<Unit> =
        withContext(Dispatchers.IO) {
            try {
                api.unforwardPort(port, projectId)
                forwards.remove(port)
                _ports.value = _ports.value.filterNot { it.port == port }
                Result.success(Unit)
            } catch (e: Exception) {
                _error.value = e.message ?: "Failed to unforward port"
                Result.failure(e)
            }
        }

    /**
     * URL to open for [port]: the loopback tunnel when one can be established,
     * otherwise the server's own proxy URL. Null when the port is unknown.
     */
    suspend fun openUrl(port: Int): String? = withContext(Dispatchers.IO) {
        forwards.ensure(port)?.localUrl
            ?: _ports.value.firstOrNull { it.port == port }?.url?.takeIf { it.isNotBlank() }
    }

    /** The server's own proxy URL for [port] (LAN-reachable), when listed. */
    fun lanUrl(port: Int): String? =
        _ports.value.firstOrNull { it.port == port }?.url?.takeIf { it.isNotBlank() }

    /** Drop every loopback listener (sign-out, settings change, teardown). */
    suspend fun stop() {
        forwards.clear()
    }
}
