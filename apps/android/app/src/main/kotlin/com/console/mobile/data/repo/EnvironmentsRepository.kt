package com.console.mobile.data.repo

import com.console.mobile.data.local.ChatPersistence
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.AuthStateHolder
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.Environment
import com.console.mobile.data.store.EnvironmentsState
import com.console.mobile.data.store.EnvironmentsStateHolder
import com.console.mobile.data.store.FsStateHolder
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.ProviderStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.store.TerminalStateHolder
import com.console.mobile.data.store.UsageStateHolder
import java.util.UUID

class EnvironmentsRepository(
    private val preferencesStore: PreferencesStore,
    private val environmentsState: EnvironmentsStateHolder,
    private val appState: AppStateHolder,
    private val chatState: ChatStateHolder,
    private val chatPersistence: ChatPersistence,
    private val sessionState: SessionStateHolder,
    private val projectState: ProjectStateHolder,
    private val providerState: ProviderStateHolder,
    private val authState: AuthStateHolder,
    private val fsState: FsStateHolder,
    private val usageState: UsageStateHolder,
    private val terminalState: TerminalStateHolder,
    private val onBackendUrlChanged: (String?) -> Unit = {},
) {
    init {
        val (persistedEnvs, persistedActiveId) = preferencesStore.loadEnvironments()
        environmentsState.set(
            EnvironmentsState(
                environments = persistedEnvs,
                activeId = persistedActiveId,
            ),
        )
        val activeUrl = persistedEnvs.firstOrNull { it.id == persistedActiveId }?.url
        appState.setBackendUrl(activeUrl)
    }

    fun addEnvironment(name: String, rawUrl: String): Environment {
        val trimmedName = name.trim()
        val url = rawUrl.trim().trimEnd('/')
        val currentEnvs = environmentsState.state.value.environments
        val previousUrl = currentEnvs.firstOrNull { it.id == environmentsState.state.value.activeId }?.url

        if (previousUrl != null && previousUrl != url) {
            resetServerState()
        }

        val env = Environment(
            id = "env_" + UUID.randomUUID().toString().take(6),
            name = trimmedName,
            url = url,
        )
        val next = currentEnvs + env
        environmentsState.set(
            environmentsState.state.value.copy(
                environments = next,
                activeId = env.id,
            ),
        )
        preferencesStore.saveEnvironments(next, env.id)
        applyActive(env.url)
        return env
    }

    /**
     * Point the app at a server, reusing its environment if it already has
     * one.
     *
     * Used after pairing: the pairing URL named the server, and a device
     * token is only valid on the machine that issued it, so the app has to
     * end up pointed at that machine. An existing environment for the same
     * address is reused rather than duplicated.
     */
    fun activateUrl(name: String, rawUrl: String): Environment {
        val url = rawUrl.trim().trimEnd('/')
        val current = environmentsState.state.value
        val existing = current.environments.firstOrNull { it.url.trimEnd('/') == url }
        if (existing != null) {
            activateEnvironment(existing.id)
            return existing
        }
        return addEnvironment(name, url)
    }

    /**
     * Keep a server in the list without pointing the app at it.
     *
     * The add sheet saves the server before it connects: a failed pairing
     * must not lose the entry the user typed, and the sheet itself stays
     * open until the connection actually works. An existing environment
     * for the same address is reused rather than duplicated, and neither
     * the active environment nor any server state is touched.
     */
    fun saveEnvironment(name: String, rawUrl: String): Environment {
        val url = rawUrl.trim().trimEnd('/')
        val current = environmentsState.state.value
        current.environments.firstOrNull { it.url.trimEnd('/') == url }?.let { return it }
        val env = Environment(
            id = "env_" + UUID.randomUUID().toString().take(6),
            name = name.trim(),
            url = url,
        )
        val next = current.environments + env
        environmentsState.set(current.copy(environments = next))
        preferencesStore.saveEnvironments(next, current.activeId)
        return env
    }

    fun activateEnvironment(id: String) {
        val current = environmentsState.state.value
        val target = current.environments.firstOrNull { it.id == id } ?: return
        val previousUrl = current.environments.firstOrNull { it.id == current.activeId }?.url
        val isDifferent = previousUrl != null && previousUrl != target.url

        if (isDifferent) {
            resetServerState()
        }

        environmentsState.set(current.copy(activeId = id))
        preferencesStore.saveEnvironments(current.environments, id)
        applyActive(target.url)
    }

    fun removeEnvironment(id: String) {
        val current = environmentsState.state.value
        val wasActive = current.activeId == id
        val nextEnvs = current.environments.filter { it.id != id }
        val nextActiveId = if (wasActive) nextEnvs.firstOrNull()?.id else current.activeId

        if (wasActive) {
            resetServerState()
        }

        environmentsState.set(
            current.copy(
                environments = nextEnvs,
                activeId = nextActiveId,
                probes = current.probes - id,
            ),
        )
        preferencesStore.saveEnvironments(nextEnvs, nextActiveId)
        val nextUrl = nextEnvs.firstOrNull { it.id == nextActiveId }?.url
        applyActive(nextUrl)
    }

    fun updateEnvironment(id: String, name: String? = null, url: String? = null) {
        val current = environmentsState.state.value
        val old = current.environments.firstOrNull { it.id == id } ?: return
        val newUrl = url?.trim()?.trimEnd('/') ?: old.url
        val newName = name?.trim() ?: old.name
        val isActive = current.activeId == id
        val urlChanged = isActive && newUrl != old.url

        if (urlChanged) {
            resetServerState()
        }

        val updated = current.environments.map {
            if (it.id == id) it.copy(name = newName, url = newUrl) else it
        }
        environmentsState.set(current.copy(environments = updated))
        preferencesStore.saveEnvironments(updated, current.activeId)
        if (isActive) {
            applyActive(newUrl)
        }
    }

    fun deactivate() {
        resetServerState()
        preferencesStore.saveEnvironments(emptyList(), null)
        preferencesStore.backendUrl = null
        environmentsState.set(EnvironmentsState(environments = emptyList(), activeId = null))
        applyActive(null)
    }

    fun resetServerState() {
        chatState.clearAll()
        chatPersistence.clear()
        sessionState.clearAll()
        projectState.clear()
        providerState.clear()
        authState.patch { it.copy(status = null, projectIds = emptyMap(), error = null) }
        fsState.patch { it.copy(treesByPath = emptyMap(), fileContentsByPath = emptyMap(), busyPaths = emptySet(), error = null) }
        usageState.invalidate()
        terminalState.clear()
        appState.clearSelections()
    }

    private fun applyActive(url: String?) {
        appState.setBackendUrl(url)
        onBackendUrlChanged(url)
    }
}
