package com.console.mobile.data.api

import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.local.TokenStore
import okhttp3.OkHttpClient

/**
 * Port of packages/api/src/client.ts (configureConsoleApi / getConsoleApiClient).
 * Phase 0: holds clients + stores, exposes baseUrl/token resolution.
 * Phase 1: add Ktor/OkHttp interceptors, kotlinx.serialization, ConsoleApi endpoints.
 */
class ConsoleApiClient(
    val httpCallClient: OkHttpClient,
    val httpClient: OkHttpClient,
    private val preferencesStore: PreferencesStore,
    private val tokenStore: TokenStore,
) {
    val baseUrl: String
        get() = preferencesStore.backendUrl?.trim()?.trimEnd('/') ?: "http://localhost:3000"

    val authToken: String?
        get() = tokenStore.authToken
}
