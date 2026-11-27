package com.console.mobile.data.api

import com.console.mobile.core.util.reachableUrl
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
    /** The address as stored: an http(s) URL, or `console-iroh://<node-id>`. */
    open val serverUrl: String
        get() = baseUrlOverride
            ?: preferencesStore?.backendUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotEmpty() }
            ?: "http://localhost:3000"

    /**
     * The URL requests are sent to. An iroh address resolves to the loopback
     * port of its tunnel, so every HTTP, SSE and WebSocket caller works
     * unchanged; anything else is [serverUrl]. If the tunnel cannot be made
     * the iroh address is returned as is and the request fails loudly.
     */
    open val baseUrl: String
        get() {
            return reachableUrl(serverUrl)
        }

    open val authToken: String?
        get() = if (hasAuthOverride) authTokenOverride else tokenStore?.tokenFor(serverUrl) ?: authTokenOverride
}
