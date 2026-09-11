package com.console.mobile.data.store

import com.console.mobile.data.model.ApprovalModeOption
import com.console.mobile.data.model.Model
import com.console.mobile.data.model.ProviderCatalogEntry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class ProviderState(
    val providers: List<ProviderCatalogEntry> = emptyList(),
    val modelsByProvider: Map<String, List<Model>> = emptyMap(),
    val loadingProviders: Boolean = false,
    val loadingModels: Map<String, Boolean> = emptyMap(),
    val error: String? = null,
    val approvalModes: List<ApprovalModeOption> = emptyList(),
    val loadingApprovalModes: Boolean = false,
)

class ProviderStateHolder(initial: ProviderState = ProviderState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<ProviderState> = _state.asStateFlow()

    fun setProviders(v: List<ProviderCatalogEntry>) { _state.value = _state.value.copy(providers = v, loadingProviders = false, error = null) }
    fun setLoadingProviders(v: Boolean) { _state.value = _state.value.copy(loadingProviders = v) }
    fun setModels(providerId: String, models: List<Model>) {
        _state.value = _state.value.copy(modelsByProvider = _state.value.modelsByProvider + (providerId to models), loadingModels = _state.value.loadingModels + (providerId to false))
    }
    fun setLoadingModel(providerId: String, v: Boolean) { _state.value = _state.value.copy(loadingModels = _state.value.loadingModels + (providerId to v)) }
    fun setApprovalModes(v: List<ApprovalModeOption>) { _state.value = _state.value.copy(approvalModes = v, loadingApprovalModes = false) }
    fun setError(e: String?) { _state.value = _state.value.copy(error = e) }
    fun clear() { _state.value = ProviderState() }

    fun resolveProvider(modelId: String, fallback: String?): String? {
        val s = _state.value
        for (p in s.providers) {
            if ((s.modelsByProvider[p.name] ?: p.models).any { it.id == modelId }) return p.name
        }
        return fallback
    }
}
