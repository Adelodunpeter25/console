package com.console.mobile.data.repo

import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.data.local.ChatPersistence
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.ProviderAuthStatus
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UserMessage
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.AuthStateHolder
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.EnvironmentsStateHolder
import com.console.mobile.data.store.FsStateHolder
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.ProviderStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.store.TerminalRecord
import com.console.mobile.data.store.TerminalStateHolder
import com.console.mobile.data.store.TerminalStatus
import com.console.mobile.data.store.UsageStateHolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

private class FakeMemorySharedPreferences : android.content.SharedPreferences {
    val data = mutableMapOf<String, Any?>()

    override fun getAll(): Map<String, *> = data
    override fun getString(key: String, defValue: String?): String? = data[key] as? String ?: defValue
    override fun getStringSet(key: String, defValues: Set<String>?): Set<String>? = null
    override fun getInt(key: String, defValue: Int): Int = (data[key] as? Int) ?: defValue
    override fun getLong(key: String, defValue: Long): Long = (data[key] as? Long) ?: defValue
    override fun getFloat(key: String, defValue: Float): Float = (data[key] as? Float) ?: defValue
    override fun getBoolean(key: String, defValue: Boolean): Boolean = (data[key] as? Boolean) ?: defValue
    override fun contains(key: String): Boolean = data.containsKey(key)
    override fun edit(): android.content.SharedPreferences.Editor = Editor(this)
    override fun registerOnSharedPreferenceChangeListener(listener: android.content.SharedPreferences.OnSharedPreferenceChangeListener?) {}
    override fun unregisterOnSharedPreferenceChangeListener(listener: android.content.SharedPreferences.OnSharedPreferenceChangeListener?) {}

    class Editor(private val parent: FakeMemorySharedPreferences) : android.content.SharedPreferences.Editor {
        private val pending = mutableMapOf<String, Any?>()
        private var clear = false
        override fun putString(key: String, value: String?) = apply { pending[key] = value }
        override fun putStringSet(key: String, values: Set<String>?) = this
        override fun putInt(key: String, value: Int) = apply { pending[key] = value }
        override fun putLong(key: String, value: Long) = apply { pending[key] = value }
        override fun putFloat(key: String, value: Float) = apply { pending[key] = value }
        override fun putBoolean(key: String, value: Boolean) = apply { pending[key] = value }
        override fun remove(key: String) = apply { pending[key] = null }
        override fun clear() = apply { clear = true }
        override fun commit() = true.also { apply() }
        override fun apply() {
            if (clear) parent.data.clear()
            for ((k, v) in pending) if (v == null) parent.data.remove(k) else parent.data[k] = v
        }
    }
}

class EnvironmentsRepositoryTest {
    private fun fixture(): Pair<EnvironmentsRepository, FixtureHolders> {
        val prefsMemory = FakeMemorySharedPreferences()
        val chatPrefsMemory = FakeMemorySharedPreferences()
        val prefStore = PreferencesStore(androidContextStub(prefsMemory))
        val chatPersistence = ChatPersistence(chatPrefsMemory)

        val holders = FixtureHolders(
            app = AppStateHolder(),
            chat = ChatStateHolder(),
            session = SessionStateHolder(),
            project = ProjectStateHolder(),
            provider = ProviderStateHolder(),
            auth = AuthStateHolder(),
            fs = FsStateHolder(),
            usage = UsageStateHolder(),
            terminal = TerminalStateHolder(),
            envState = EnvironmentsStateHolder(),
            chatPersist = chatPersistence,
            prefStore = prefStore,
        )

        val repo = EnvironmentsRepository(
            preferencesStore = prefStore,
            environmentsState = holders.envState,
            appState = holders.app,
            chatState = holders.chat,
            chatPersistence = chatPersistence,
            sessionState = holders.session,
            projectState = holders.project,
            providerState = holders.provider,
            authState = holders.auth,
            fsState = holders.fs,
            usageState = holders.usage,
            terminalState = holders.terminal,
        )
        return repo to holders
    }

    private class FixtureHolders(
        val app: AppStateHolder,
        val chat: ChatStateHolder,
        val session: SessionStateHolder,
        val project: ProjectStateHolder,
        val provider: ProviderStateHolder,
        val auth: AuthStateHolder,
        val fs: FsStateHolder,
        val usage: UsageStateHolder,
        val terminal: TerminalStateHolder,
        val envState: EnvironmentsStateHolder,
        val chatPersist: ChatPersistence,
        val prefStore: PreferencesStore,
    )

    private fun androidContextStub(prefs: android.content.SharedPreferences): android.content.Context {
        // Minimal dynamic proxy / subclass to provide getSharedPreferences
        return object : android.content.ContextWrapper(null) {
            override fun getSharedPreferences(name: String?, mode: Int): android.content.SharedPreferences = prefs
            override fun getApplicationContext(): android.content.Context = this
        }
    }

    @Test fun addEnvironmentSetsActiveAndBackendUrl() {
        val (repo, holders) = fixture()

        val env = repo.addEnvironment("Local Server", "http://192.168.1.50:3000")

        assertEquals("Local Server", env.name)
        assertEquals("http://192.168.1.50:3000", env.url)
        assertEquals(env.id, holders.envState.state.value.activeId)
        assertEquals("http://192.168.1.50:3000", holders.app.state.value.backendUrl)
        assertEquals("http://192.168.1.50:3000", holders.prefStore.backendUrl)
    }

    @Test fun switchingEnvironmentsCascadesResetServerState() {
        val (repo, holders) = fixture()

        val env1 = repo.addEnvironment("Server 1", "http://10.0.0.1:3000")

        // Populate server-scoped state
        holders.chat.update("s1") {
            it.copy(messages = listOf(UserMessage(id = "m1", content = "hi")))
        }
        holders.chatPersist.saveNow(holders.chat.sessions.value)
        holders.session.setStatus("s1", SessionStatus.Working)
        holders.project.addProject(ProjectInfo("p1", "proj", "/path", 1, 2))
        holders.auth.patch { it.copy(status = mapOf("antigravity" to ProviderAuthStatus(loggedIn = true))) }
        holders.terminal.ensure(TerminalRecord("t1", "p1", TerminalStatus.Running))
        holders.app.setSelectedSessionId("s1")

        assertTrue(holders.chat.sessions.value.isNotEmpty())
        assertTrue(holders.chatPersist.load().isNotEmpty())
        assertTrue(holders.project.state.value.projects.isNotEmpty())
        assertEquals("s1", holders.app.state.value.selectedSessionId)

        // Add second environment -> automatically switches -> triggers resetServerState
        val env2 = repo.addEnvironment("Server 2", "http://10.0.0.2:3000")

        assertEquals(env2.id, holders.envState.state.value.activeId)
        assertEquals("http://10.0.0.2:3000", holders.app.state.value.backendUrl)

        // Every server-scoped bucket must be completely empty now
        assertTrue(holders.chat.sessions.value.isEmpty())
        assertTrue(holders.chatPersist.load().isEmpty())
        assertTrue(holders.session.statuses.value.isEmpty())
        assertTrue(holders.project.state.value.projects.isEmpty())
        assertNull(holders.auth.state.value.status)
        assertTrue(holders.terminal.terminals.value.isEmpty())
        assertNull(holders.app.state.value.selectedSessionId)
    }

    @Test fun deactivateClearsEverything() {
        val (repo, holders) = fixture()
        repo.addEnvironment("Server", "http://1.2.3.4:3000")
        holders.app.setSelectedProjectId("p1")

        repo.deactivate()

        assertTrue(holders.envState.state.value.environments.isEmpty())
        assertNull(holders.envState.state.value.activeId)
        assertNull(holders.app.state.value.backendUrl)
        assertNull(holders.app.state.value.selectedProjectId)
    }
}
