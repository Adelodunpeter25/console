package com.console.mobile

import android.content.Context
import com.console.mobile.core.notification.LocalNotificationPresenter
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.local.TokenStore
import java.util.concurrent.TimeUnit
import okhttp3.OkHttpClient

/**
 * Application-wide services — mirrors Remodex AppContainer (object + initialize).
 * Phase 0: minimal. Phase 1 will add SessionPersistence, repositories, etc.
 * No Hilt for v1 (see plan §5.5); manual container keeps migration incremental.
 */
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

    fun initialize(context: Context) {
        val app = context.applicationContext
        appContext = app
        httpCallClient =
            OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .writeTimeout(20, TimeUnit.SECONDS)
                .callTimeout(30, TimeUnit.SECONDS)
                .retryOnConnectionFailure(false)
                .build()
        httpClient =
            httpCallClient.newBuilder()
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
        LocalNotificationPresenter.ensureChannelCreated(app)
    }
}
