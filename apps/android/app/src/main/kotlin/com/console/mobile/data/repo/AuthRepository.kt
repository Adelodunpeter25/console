package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.LoginUrlResult
import com.console.mobile.data.model.AuthStatusShim
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.OAuthLoginUrlDto
import com.console.mobile.data.model.ProviderAuthStatus
import com.console.mobile.data.store.AuthState
import com.console.mobile.data.store.AuthStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class AuthRepository(
    private val api: ConsoleApi,
    private val authState: AuthStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    fun loadStatus() {
        scope.launch {
            authState.patch { it.copy(loading = true, error = null) }
            try {
                val shim = withContext(Dispatchers.IO) { api.getAuthStatus() }
                val statusMap = mapOf(
                    "antigravity" to shim.antigravity,
                    "codex" to shim.codex,
                    "devin" to shim.devin,
                )
                val projectIds = mapOf(
                    "antigravity" to shim.antigravity.configuredProjectId,
                )
                authState.set(
                    AuthState(
                        status = statusMap,
                        loading = false,
                        error = null,
                        projectIds = projectIds,
                    ),
                )
            } catch (e: Exception) {
                authState.set(
                    AuthState(
                        status = mapOf(
                            "antigravity" to ProviderAuthStatus(loggedIn = false),
                            "codex" to ProviderAuthStatus(loggedIn = false),
                            "devin" to ProviderAuthStatus(loggedIn = false),
                        ),
                        loading = false,
                        error = e.message ?: "Failed to load auth status",
                    ),
                )
            }
        }
    }

    suspend fun getLoginUrl(provider: String): LoginUrlResult =
        withContext(Dispatchers.IO) {
            api.getLoginUrl(OAuthLoginUrlDto(provider = provider))
        }

    suspend fun submitCallback(provider: String, code: String, state: String? = null) {
        withContext(Dispatchers.IO) {
            api.handleCallback(OAuthCallbackDto(provider = provider, code = code, state = state))
        }
        loadStatus()
    }

    suspend fun saveProjectId(provider: String, projectId: String?) {
        authState.patch { it.copy(savingProjectId = true, error = null) }
        try {
            withContext(Dispatchers.IO) {
                api.saveProjectId(provider, projectId?.trim()?.ifEmpty { null })
            }
            authState.patch {
                it.copy(
                    savingProjectId = false,
                    projectIds = it.projectIds + (provider to projectId?.trim()?.ifEmpty { null }),
                )
            }
        } catch (e: Exception) {
            authState.patch {
                it.copy(
                    savingProjectId = false,
                    error = e.message ?: "Failed to save project ID",
                )
            }
            throw e
        }
    }

    fun reset() {
        authState.patch { it.copy(loggingIn = null, error = null) }
    }
}
