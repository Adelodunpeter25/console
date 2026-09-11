package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.UsageReport
import com.console.mobile.data.store.UsageStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class UsageRepository(
    private val api: ConsoleApi,
    private val usageState: UsageStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    companion object {
        const val STALE_MS = 30_000L
    }

    fun loadAllUsage(force: Boolean = false) {
        val last = usageState.state.value.lastFetchedAt
        val isStale = last == null || (System.currentTimeMillis() - last) > STALE_MS
        if (!force && !isStale && usageState.state.value.reports.isNotEmpty()) return

        scope.launch {
            usageState.patch { it.copy(loading = true, error = null) }
            try {
                val reports = withContext(Dispatchers.IO) { api.getAllUsage() }
                usageState.set(
                    usageState.state.value.copy(
                        reports = reports,
                        loading = false,
                        error = null,
                        lastFetchedAt = System.currentTimeMillis(),
                    ),
                )
            } catch (e: Exception) {
                usageState.patch { it.copy(loading = false, error = e.message ?: "Failed to load usage") }
            }
        }
    }

    fun loadUsage(providerId: String) {
        val cached = usageState.state.value.reports[providerId]
        val last = usageState.state.value.lastFetchedAt
        val isStale = last == null || (System.currentTimeMillis() - last) > STALE_MS
        if (cached != null && !isStale) return

        scope.launch {
            usageState.patch {
                it.copy(loadingByProvider = it.loadingByProvider + (providerId to true))
            }
            try {
                val report = withContext(Dispatchers.IO) { api.getProviderUsage(providerId) }
                usageState.patch {
                    it.copy(
                        reports = it.reports + (providerId to report),
                        loadingByProvider = it.loadingByProvider + (providerId to false),
                        lastFetchedAt = System.currentTimeMillis(),
                    )
                }
            } catch (e: Exception) {
                usageState.patch {
                    it.copy(
                        loadingByProvider = it.loadingByProvider + (providerId to false),
                        error = e.message ?: "Failed to load usage for $providerId",
                    )
                }
            }
        }
    }

    fun invalidate(providerId: String? = null) {
        usageState.invalidate(providerId)
    }
}
