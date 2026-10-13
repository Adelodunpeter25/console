package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.store.IntegrationsStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class IntegrationsRepository(
    private val api: ConsoleApi,
    private val integrationsState: IntegrationsStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    fun loadIntegrations() {
        scope.launch {
            integrationsState.patch { it.copy(loading = true, error = null) }
            try {
                val response = withContext(Dispatchers.IO) { api.getIntegrations() }
                integrationsState.patch { it.copy(integrations = response.integrations, loading = false, error = null) }
            } catch (e: Exception) {
                // Keep the last known rows. Clearing them on a transient
                // failure would flash every installed binary as missing.
                integrationsState.patch { it.copy(loading = false, error = e.message ?: "Failed to load integrations") }
            }
        }
    }

    /**
     * Install gh the official way for the server's platform. Shares the
     * refresh busy/error flags: one in-flight operation at a time, and a
     * failed install reads like a failed refresh. Success replaces the rows,
     * so the button flips to Installed with no extra fetch.
     */
    fun installGh() {
        scope.launch {
            integrationsState.patch { it.copy(loading = true, error = null) }
            try {
                val response = withContext(Dispatchers.IO) { api.installGh() }
                integrationsState.patch { it.copy(integrations = response.integrations, loading = false, error = null) }
            } catch (e: Exception) {
                integrationsState.patch { it.copy(loading = false, error = e.message ?: "Failed to install GitHub CLI") }
            }
        }
    }
}
