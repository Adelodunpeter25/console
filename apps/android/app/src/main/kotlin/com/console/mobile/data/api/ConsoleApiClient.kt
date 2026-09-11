package com.console.mobile.data.api

import com.console.mobile.data.local.PreferencesStore
import com.console.mobile.data.local.TokenStore
import okhttp3.OkHttpClient

open class ConsoleApiClient(
    val httpCallClient: OkHttpClient,
    val httpClient: OkHttpClient,
    private val preferencesStore: PreferencesStore? = null,
    private val tokenStore: TokenStore? = null,
    private val baseUrlOverride: String? = null,
    private val authTokenOverride: String? = null,
    private val hasAuthOverride: Boolean = false,
) {
    open val baseUrl: String
        get() = baseUrlOverride
            ?: preferencesStore?.backendUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotEmpty() }
            ?: "http://localhost:3000"

    open val authToken: String?
        get() = if (hasAuthOverride) authTokenOverride else tokenStore?.authToken ?: authTokenOverride
}
