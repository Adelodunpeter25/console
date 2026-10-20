package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.local.TokenStore
import console.v1.AuthStatusResponse
import console.v1.GitHubAuthStatus
import console.v1.OAuthLoginUrlResponse
import console.v1.ProviderAuthStatus
import console.v1.ProviderCredentialStatus
import com.console.mobile.data.model.DeviceInfo
import com.console.mobile.data.model.GitHubTokenDto
import com.console.mobile.data.model.OAuthCallbackDto
import com.console.mobile.data.model.PairingData
import com.console.mobile.data.model.PairingGrantData
import com.console.mobile.data.model.PairingGrantRequest
import com.console.mobile.data.model.PairingRequest
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
    private val tokenStore: TokenStore,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    /**
     * Exchange a pairing grant for a device token and persist it against the
     * server it came from. Pairing is per-server: the same app can be paired
     * to several environments without their tokens overwriting each other.
     */
    suspend fun pair(serverUrl: String, request: PairingRequest): PairingData = withContext(Dispatchers.IO) {
        val paired = api.pair(request)
        // The device is optional in the schema, so it is read defensively:
        // the token is what the client actually needs.
        val device = paired.device
        tokenStore.save(
            serverUrl,
            com.console.mobile.data.local.StoredServerAuth(
                token = paired.token,
                deviceId = device?.id.orEmpty(),
                deviceName = device?.name.orEmpty(),
                expiresAt = paired.expires_at,
            ),
        )
        paired
    }

    /** Forget one server's device credential (e.g. the server revoked us). */
    suspend fun unpair(serverUrl: String) = withContext(Dispatchers.IO) {
        tokenStore.clear(serverUrl)
    }

    suspend fun createPairingGrant(request: PairingGrantRequest): PairingGrantData = withContext(Dispatchers.IO) {
        api.createPairingGrant(request)
    }

    suspend fun listDevices(): List<DeviceInfo> = withContext(Dispatchers.IO) { api.listAuthDevices() }

    suspend fun revokeDevice(id: String) = withContext(Dispatchers.IO) { api.revokeAuthDevice(id) }

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
                        // `connected` has explicit presence on
                        // the wire, so the not-reported default is null; every
                        // reader treats null as "not connected".
                        github = status.github ?: GitHubAuthStatus(),
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
            authState.patch { it.copy(github = GitHubAuthStatus(), githubBusy = false) }
        } catch (e: Exception) {
            authState.patch { it.copy(githubBusy = false) }
            throw e
        }
    }

    /**
     * Read which providers have an API key stored. The response reports only
     * that a key exists and where it came from — the key itself lives on the
     * server and no endpoint will hand it back.
     */
    suspend fun loadProviderCredentials() {
        try {
            val statuses = withContext(Dispatchers.IO) { api.getProviderCredentials() }
            authState.patch { it.copy(providerCredentials = statuses) }
        } catch (e: Exception) {
            // Status is decorative: a failure here must not block the page, and
            // a provider with no entry simply renders as "not connected".
            authState.patch { it.copy(error = e.message ?: "Couldn't read provider credentials") }
        }
    }

    /**
     * Store an API key for a provider on the server.
     *
     * The key is handed straight to the server and dropped — keeping it would
     * mean holding the user's provider secret in app state for no benefit,
     * since nothing on this side ever needs to read it back.
     */
    suspend fun setProviderCredential(provider: String, apiKey: String) {
        authState.patch { it.copy(credentialsBusy = it.credentialsBusy + provider) }
        try {
            val response = withContext(Dispatchers.IO) { api.setProviderCredential(provider, apiKey) }
            authState.patch {
                it.copy(
                    credentialsBusy = it.credentialsBusy - provider,
                    providerCredentials = it.providerCredentials + (
                        provider to ProviderCredentialStatus(
                            configured = response.configured ?: true,
                            source = response.source,
                        )
                        ),
                )
            }
        } catch (e: Exception) {
            authState.patch { it.copy(credentialsBusy = it.credentialsBusy - provider) }
            throw e
        }
    }

    /** Remove a provider's stored key, falling back to whatever the server did before. */
    suspend fun clearProviderCredential(provider: String) {
        authState.patch { it.copy(credentialsBusy = it.credentialsBusy + provider) }
        try {
            val response = withContext(Dispatchers.IO) { api.clearProviderCredential(provider) }
            authState.patch {
                it.copy(
                    credentialsBusy = it.credentialsBusy - provider,
                    providerCredentials = it.providerCredentials + (
                        provider to ProviderCredentialStatus(
                            configured = response.configured ?: false,
                            source = response.source,
                        )
                        ),
                )
            }
        } catch (e: Exception) {
            authState.patch { it.copy(credentialsBusy = it.credentialsBusy - provider) }
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
