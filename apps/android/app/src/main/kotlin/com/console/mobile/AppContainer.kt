package com.console.mobile

import android.content.Context
import com.console.mobile.core.notification.LocalNotificationPresenter
import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.HttpTransport
import com.console.mobile.data.api.OkHttpConsoleApi
import com.console.mobile.data.local.ChatPersistence
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.local.TokenStore
import com.console.mobile.data.repo.AuthRepository
import com.console.mobile.data.repo.ChatRepository
import com.console.mobile.data.repo.EnvironmentsRepository
import com.console.mobile.data.repo.FsRepository
import com.console.mobile.data.repo.GitRepository
import com.console.mobile.data.repo.NotificationRepository
import com.console.mobile.data.repo.ProjectRepository
import com.console.mobile.data.repo.ProviderRepository
import com.console.mobile.data.repo.SessionRepository
import com.console.mobile.data.repo.TerminalRepository
import com.console.mobile.data.repo.UsageRepository
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.AuthStateHolder
import com.console.mobile.data.store.ChatStateHolder
import com.console.mobile.data.store.EnvironmentsStateHolder
import com.console.mobile.data.store.FsStateHolder
import com.console.mobile.data.store.MobileTab
import com.console.mobile.data.store.ProjectStateHolder
import com.console.mobile.data.store.ProviderStateHolder
import com.console.mobile.data.store.SessionStateHolder
import com.console.mobile.data.store.TerminalStateHolder
import com.console.mobile.data.store.UsageStateHolder
import com.console.mobile.data.stream.ChatStreamClient
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient

object AppContainer {
    private val pendingNotificationLock = Any()

    @Volatile
    private var pendingNotificationId: String? = null

    fun setPendingOpenFromNotification(id: String?) {
        val v = id?.trim()?.takeIf { it.isNotEmpty() } ?: return
        synchronized(pendingNotificationLock) { pendingNotificationId = v }
    }

    fun consumePendingOpenFromNotification(): String? =
        synchronized(pendingNotificationLock) {
            val v = pendingNotificationId
            pendingNotificationId = null
            v
        }

    lateinit var appContext: Context
        private set

    lateinit var httpCallClient: OkHttpClient
        private set

    lateinit var httpClient: OkHttpClient
        private set

    lateinit var preferencesStore: PreferencesStore
        private set

    lateinit var tokenStore: TokenStore
        private set

    lateinit var consoleApiClient: ConsoleApiClient
        private set

    lateinit var consoleApi: ConsoleApi
        private set

    lateinit var appStateHolder: AppStateHolder
        private set

    lateinit var chatStateHolder: ChatStateHolder
        private set

    lateinit var sessionStateHolder: SessionStateHolder
        private set

    lateinit var projectStateHolder: ProjectStateHolder
        private set

    lateinit var providerStateHolder: ProviderStateHolder
        private set

    lateinit var authStateHolder: AuthStateHolder
        private set

    lateinit var fsStateHolder: FsStateHolder
        private set

    lateinit var usageStateHolder: UsageStateHolder
        private set

    lateinit var terminalStateHolder: TerminalStateHolder
        private set

    lateinit var environmentsStateHolder: EnvironmentsStateHolder
        private set

    lateinit var chatPersistence: ChatPersistence
        private set

    lateinit var chatRepository: ChatRepository
        private set

    lateinit var sessionRepository: SessionRepository
        private set

    lateinit var projectRepository: ProjectRepository
        private set

    lateinit var providerRepository: ProviderRepository
        private set

    lateinit var fsRepository: FsRepository
        private set

    lateinit var usageRepository: UsageRepository
        private set

    lateinit var gitRepository: GitRepository
        private set

    lateinit var terminalRepository: TerminalRepository
        private set

    lateinit var notificationRepository: NotificationRepository
        private set

    lateinit var authRepository: AuthRepository
        private set

    lateinit var environmentsRepository: EnvironmentsRepository
        private set

    fun initialize(context: Context) {
        val app = context.applicationContext
        appContext = app

        httpCallClient = OkHttpClient.Builder()
            .connectTimeout(10, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .writeTimeout(20, TimeUnit.SECONDS)
            .callTimeout(30, TimeUnit.SECONDS)
            .retryOnConnectionFailure(false)
            .build()

        httpClient = httpCallClient.newBuilder()
            .pingInterval(30, TimeUnit.SECONDS)
            .readTimeout(0, TimeUnit.SECONDS)
            .callTimeout(0, TimeUnit.SECONDS)
            .retryOnConnectionFailure(true)
            .build()

        preferencesStore = PreferencesStore(app)
        tokenStore = TokenStore(app)

        consoleApiClient = ConsoleApiClient(
            httpCallClient = httpCallClient,
            httpClient = httpClient,
            preferencesStore = preferencesStore,
            tokenStore = tokenStore,
        )

        val transport = HttpTransport(
            callClient = httpCallClient,
            baseUrl = { consoleApiClient.baseUrl },
            authToken = { consoleApiClient.authToken },
        )
        consoleApi = OkHttpConsoleApi(transport)

        appStateHolder = AppStateHolder()
        chatStateHolder = ChatStateHolder()
        sessionStateHolder = SessionStateHolder()
        projectStateHolder = ProjectStateHolder()
        providerStateHolder = ProviderStateHolder()
        authStateHolder = AuthStateHolder()
        fsStateHolder = FsStateHolder()
        usageStateHolder = UsageStateHolder()
        terminalStateHolder = TerminalStateHolder()
        environmentsStateHolder = EnvironmentsStateHolder()

        chatPersistence = ChatPersistence.create(app)
        val streamClient = ChatStreamClient(httpClient)

        projectRepository = ProjectRepository(
            api = consoleApi,
            projectState = projectStateHolder,
            sessionState = sessionStateHolder,
            appState = appStateHolder,
        )
        providerRepository = ProviderRepository(
            api = consoleApi,
            providerState = providerStateHolder,
        )
        fsRepository = FsRepository(
            api = consoleApi,
            fsState = fsStateHolder,
        )
        usageRepository = UsageRepository(
            api = consoleApi,
            usageState = usageStateHolder,
        )
        gitRepository = GitRepository(
            api = consoleApi,
            apiClient = consoleApiClient,
            httpClient = httpClient,
        )
        terminalRepository = TerminalRepository(
            apiClient = consoleApiClient,
            httpClient = httpClient,
            terminalState = terminalStateHolder,
        )
        notificationRepository = NotificationRepository(
            apiClient = consoleApiClient,
            httpClient = httpClient,
        )

        chatRepository = ChatRepository(
            api = consoleApi,
            apiClient = consoleApiClient,
            streamClient = streamClient,
            chats = chatStateHolder,
            sessions = sessionStateHolder,
            persistence = chatPersistence,
            providerRepo = providerRepository,
        )
        sessionRepository = SessionRepository(
            api = consoleApi,
            sessions = sessionStateHolder,
            chats = chatStateHolder,
            chatRepo = chatRepository,
        )
        authRepository = AuthRepository(
            api = consoleApi,
            authState = authStateHolder,
        )
        environmentsRepository = EnvironmentsRepository(
            preferencesStore = preferencesStore,
            environmentsState = environmentsStateHolder,
            appState = appStateHolder,
            chatState = chatStateHolder,
            chatPersistence = chatPersistence,
            sessionState = sessionStateHolder,
            projectState = projectStateHolder,
            providerState = providerStateHolder,
            authState = authStateHolder,
            fsState = fsStateHolder,
            usageState = usageStateHolder,
            terminalState = terminalStateHolder,
            onBackendUrlChanged = {
                projectRepository.loadProjects()
                projectRepository.loadSessions()
                providerRepository.loadProviders()
                providerRepository.loadApprovalModes()
                usageRepository.loadAllUsage()
                sessionRepository.refresh()
                authRepository.loadStatus()
            },
        )

        LocalNotificationPresenter.ensureChannelCreated(app)

        val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        appScope.launch {
            try {
                notificationRepository.notifications().collect { event ->
                    val viewingSame = appStateHolder.state.value.activeTab == MobileTab.Chat &&
                        appStateHolder.state.value.selectedSessionId == event.sessionId
                    if (!viewingSame) {
                        LocalNotificationPresenter.showNotification(
                            app,
                            event.title,
                            event.body,
                            event.sessionId,
                        )
                    }
                }
            } catch (_: Exception) {
            }
        }
    }
}
