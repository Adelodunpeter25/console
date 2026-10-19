package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.store.EnvironmentsStateHolder
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
     * Open the panel if this server has never had it dismissed.
     *
     * Called on every environment switch. Cheap when the server is already
     * dismissed — it returns without touching the network.
     */
    fun maybeShow(envId: String?) {
        val id = envId ?: return
        if (preferencesStore.loadOnboardingDismissed().contains(id)) {
            setupState.close()
            return
        }
        val name = environmentsState.state.value.environments
            .firstOrNull { it.id == id }?.name
        setupState.open(id, name)
        refresh()
    }

    /**
     * Read the three live facts behind the panel's steps.
     *
     * Each failure is confined to its own step: a server with no GitHub token
     * still answers the status call, but if that call is unreachable the GitHub
     * row reads as unmet rather than the panel failing to render. An onboarding
     * panel that errors out teaches the user nothing.
     *
     * Reads the API directly rather than reading existing holders, because the
     * panel can be the first thing to touch a freshly connected server and every
     * holder would still be empty.
     */
    fun refresh() {
        scope.launch {
            val projects = async { runCatching { api.getProjects() }.getOrNull() }
            val providers = async { runCatching { api.getProviderCredentials() }.getOrNull() }
            val github = async { runCatching { api.getGitHubStatus() }.getOrNull() }

            // `getProjects` is not wrapped in withContext here on purpose: the
            // OkHttp call is already off the main thread inside the API layer,
            // and awaiting three of them concurrently keeps the panel to one
            // round-trip's latency instead of three.
            val hasProject = projects.await()?.isNotEmpty() == true
            val hasProvider = providers.await()?.values?.any { it.configured == true } == true
            val hasGithub = github.await()?.connected == true

            setupState.resolve(hasProject, hasProvider, hasGithub)
        }
    }

    /**
     * Dismiss the panel for the active server.
     *
     * Latching and per server: dismissing on one server leaves every other
     * server's bit alone, so a remote box that has been set up stays quiet when
     * a newly added server is still bare.
     */
    fun dismiss() {
        setupState.state.value.activeEnvId?.let { preferencesStore.dismissOnboarding(it) }
        setupState.close()
    }
}