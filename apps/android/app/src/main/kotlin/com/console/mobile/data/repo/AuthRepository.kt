package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import console.v1.AuthStatusResponse
import console.v1.GitHubAuthStatus
import console.v1.OAuthLoginUrlResponse
import console.v1.ProviderAuthStatus
import com.console.mobile.data.model.GitHubTokenDto
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
                    this ?: ProviderAuthStatus(logged_in = false)
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
                authState.patch {
                    it.copy(
                        status = statusMap,
                        loading = false,
                        error = null,
                        projectIds = projectIds,
                        // Local-only on the server: a revoked token still reads
                        // connected until the next git failure or re-login.
                        github = status.github ?: GitHubAuthStatus(connected = false),
                    )
                }
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

    /**
     * Validate and store a GitHub personal access token so the server's git
     * (agent runs and terminals) can reach private repositories. The server
     * answers with the username only; the token is never kept here. Throws the
     * server's message on failure (invalid token, GitHub unreachable).
     */
    suspend fun connectGitHub(token: String) {
        authState.patch { it.copy(githubBusy = true) }
        try {
            val status = withContext(Dispatchers.IO) { api.connectGitHub(GitHubTokenDto(token.trim())) }
            authState.patch { it.copy(github = status, githubBusy = false) }
        } catch (e: Exception) {
            authState.patch { it.copy(githubBusy = false) }
            throw e
        }
    }

    /** Remove the stored GitHub token from the server. */
    suspend fun disconnectGitHub() {
        authState.patch { it.copy(githubBusy = true) }
        try {
            withContext(Dispatchers.IO) { api.disconnectGitHub() }
            authState.patch { it.copy(github = GitHubAuthStatus(connected = false), githubBusy = false) }
        } catch (e: Exception) {
            authState.patch { it.copy(githubBusy = false) }
            throw e
        }
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
