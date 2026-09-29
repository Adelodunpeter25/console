package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.Model
import com.console.mobile.data.model.ProviderCatalogEntry
import com.console.mobile.data.store.ProviderStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class ProviderRepository(
    private val api: ConsoleApi,
    private val providerState: ProviderStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    fun loadProviders() {
        scope.launch {
            providerState.setLoadingProviders(true)
            providerState.setError(null)
            try {
                val list = withContext(Dispatchers.IO) { api.getProviders() }
                providerState.setProviders(list)
            } catch (e: Exception) {
                providerState.setLoadingProviders(false)
                providerState.setError(e.message ?: "Failed to load providers")
            }
        }
    }

    suspend fun loadModels(providerId: String): List<Model> {
        val cached = providerState.state.value.modelsByProvider[providerId]
        if (cached != null) return cached

        providerState.setLoadingModel(providerId, true)
        providerState.setError(null)
        return try {
            val models = withContext(Dispatchers.IO) { api.getProviderModels(providerId) }
            providerState.setModels(providerId, models)
            models
        } catch (e: Exception) {
            providerState.setLoadingModel(providerId, false)
            providerState.setError(e.message ?: "Failed to load models")
            emptyList()
        }
    }

    fun loadApprovalModes() {
        if (providerState.state.value.approvalModes.isNotEmpty()) return
        scope.launch {
            try {
                val modes = withContext(Dispatchers.IO) { api.getApprovalModes() }
                providerState.setApprovalModes(modes)
            } catch (_: Exception) {
            }
        }
    }

    fun loadSettings() {
        if (providerState.state.value.loadingRoles) return
        scope.launch {
            providerState.setLoadingRoles(true)
            try {
                val settings = withContext(Dispatchers.IO) { api.getSettings() }
                providerState.setModelRoles(settings.modelRoles.toRefMap())
            } catch (e: Exception) {
                providerState.setLoadingRoles(false)
                providerState.setRolesError(e.message ?: "Failed to load settings")
            }
        }
    }

    /** PATCH the given roles; a null reference clears that role. */
    suspend fun saveModelRoles(roles: Map<String, String?>): Boolean {
        providerState.setSavingRoles(true)
        return try {
            val settings = withContext(Dispatchers.IO) { api.updateModelRoles(roles) }
            providerState.setModelRoles(settings.modelRoles.toRefMap())
            providerState.setSavingRoles(false)
            true
        } catch (e: Exception) {
            providerState.setRolesError(e.message ?: "Failed to save model roles")
            false
        }
    }

    fun resolveProvider(modelId: String, fallback: String? = null): String? =
        providerState.resolveProvider(modelId, fallback)

    fun supportsImages(providerId: String?, modelId: String?): Boolean {
        if (providerId.isNullOrBlank() || modelId.isNullOrBlank()) return true
        val models = providerState.state.value.modelsByProvider[providerId]
            ?: providerState.state.value.providers.firstOrNull { it.name == providerId }?.models
            ?: return true
        val model = models.firstOrNull { it.id == modelId } ?: return true
        return model.supportsImages
    }
}
