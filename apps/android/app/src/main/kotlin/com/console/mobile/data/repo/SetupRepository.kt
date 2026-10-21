package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.store.EnvironmentsStateHolder
import com.console.mobile.data.store.SetupState
import com.console.mobile.data.store.SetupStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.launch

/**
 * Decides whether the active server needs the first-run setup panel, and closes
 * it once dismissed.
 *
 * The panel is a one-shot pointer per server, not a progress tracker: engaging
 * a step dismisses it exactly like the Skip button does, because every step is
 * reachable from Settings afterwards and leaving the panel up over the screen it
 * just opened would be worse than closing it.
 */
class SetupRepository(
    private val api: ConsoleApi,
    private val preferencesStore: PreferencesStore,
    private val environmentsState: EnvironmentsStateHolder,
    private val setupState: SetupStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    /**
     * Open the panel, but only if this server actually has something left to set
     * up.
     *
     * The check happens *before* opening, not after. Showing the panel and then
     * hiding it a moment later when the server turns out to be complete is worse
     * than never showing it — the user still sees a first-run screen for a server
     * they configured weeks ago.
     *
     * Nothing is persisted for an already-complete server. It stays eligible for
     * the check next launch, so removing the last provider key brings the panel
     * back rather than leaving a server with nothing configured and no prompt.
     */
    fun maybeShow(envId: String?) {
        val id = envId ?: return
        // Dismissed means dismissed, for every server: this is a first-run
        // screen, and it does not come back.
        if (preferencesStore.isOnboardingDismissed()) {
            setupState.close()
            return
        }
        val name = environmentsState.state.value.environments.firstOrNull { it.id == id }?.name
        scope.launch {
            val status = fetchStatus()
            if (status.isComplete) {
                setupState.close()
            } else {
                setupState.open(id, name)
                setupState.resolve(status.hasProject, status.hasProvider, status.hasGithub)
            }
        }
    }

    /** The three live facts, each isolated so one failure cannot blank the rest. */
    private suspend fun fetchStatus(): SetupState = SetupState(
        hasProject = runCatching { api.getProjects() }.getOrNull()?.isNotEmpty() == true,
        hasProvider = runCatching { api.getProviderCredentials() }.getOrNull()
            ?.values?.any { it.configured == true } == true,
        hasGithub = runCatching { api.getGitHubStatus() }.getOrNull()?.connected == true,
    )

    /**
     * Dismiss the panel. It never comes back.
     */
    fun dismiss() {
        preferencesStore.dismissOnboardingForever()
        setupState.close()
    }
}