package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import console.v1.AuthStatusResponse
import console.v1.OAuthLoginUrlResponse
import console.v1.ProviderAuthStatus
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.OAuthLoginUrlDto
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
                val status = withContext(Dispatchers.IO) { api.getAuthStatus() }
                // Provider sub-messages are optional in proto; the server always
                // sends all four, but an absent one must still read as logged
                // out rather than crash the settings page.
                fun ProviderAuthStatus?.orLoggedOut(): ProviderAuthStatus =
                    this ?: ProviderAuthStatus.Builder().setLoggedIn(false).build()
                val antigravity = status.antigravity.orLoggedOut()
                val statusMap = mapOf(
                    "antigravity" to antigravity,
                    "codex" to status.codex.orLoggedOut(),
                    "devin" to status.devin.orLoggedOut(),
                    "claude" to status.claude.orLoggedOut(),
                )
                val projectIds = mapOf(
                    "antigravity" to antigravity.configured_project_id,
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
                // Keep the last known status/projectIds. Overwriting them with
                // all-loggedOut made one transient 500 look like every provider
                // had been disconnected, and discarded a saved antigravity project.
                authState.patch { it.copy(loading = false, error = e.message ?: "Failed to load auth status") }
            }
        }
    }

    suspend fun getLoginUrl(provider: String): OAuthLoginUrlResponse =
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
