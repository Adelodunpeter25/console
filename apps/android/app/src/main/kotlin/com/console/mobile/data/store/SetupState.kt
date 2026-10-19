package com.console.mobile.data.store

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/**
 * First-run setup panel state, mirroring the desktop's model.
 *
 * Two kinds of state, kept apart on purpose:
 *
 *  - **Stored** — one bit per server: has this person seen the panel? A fact
 *    about the human that no endpoint can answer, so it lives in
 *    [com.console.mobile.data.local.PreferencesStore] keyed by environment id.
 *  - **Derived** — is this server actually set up? Read live from the server
 *    every time the panel opens.
 *
 * Nothing about server setup is persisted. If it were, the two would drift the
 * first time a user deleted their last project: the panel would either claim
 * everything is done over an empty server, or re-open itself at someone who
 * already did the work.
 */
data class SetupState(
    /** Environment id whose panel is open, or null when none is. */
    val activeEnvId: String? = null,
    /** Human name of [activeEnvId], for the panel heading. */
    val serverName: String? = null,
    /** The server has at least one project. */
    val hasProject: Boolean = false,
    /** At least one provider has an API key stored. */
    val hasProvider: Boolean = false,
    /** A GitHub personal access token is stored. */
    val hasGithub: Boolean = false,
    /** True while the three checks are in flight. */
    val loading: Boolean = false,
) {
    /** Every step satisfied — the panel has nothing left to offer. */
    val isComplete: Boolean get() = hasProject && hasProvider && hasGithub
}

class SetupStateHolder(initial: SetupState = SetupState()) {
    private val _state = MutableStateFlow(initial)
    val state: StateFlow<SetupState> = _state.asStateFlow()

    fun open(envId: String, serverName: String?) {
        _state.value = SetupState(
            activeEnvId = envId,
            serverName = serverName,
            loading = true,
        )
    }

    fun patch(update: (SetupState) -> SetupState) {
        _state.update(update)
    }

    fun resolve(hasProject: Boolean, hasProvider: Boolean, hasGithub: Boolean) {
        _state.update { it.copy(hasProject = hasProject, hasProvider = hasProvider, hasGithub = hasGithub, loading = false) }
    }

    /** Close the panel. The caller persists the dismissal. */
    fun close() {
        _state.value = SetupState()
    }
}