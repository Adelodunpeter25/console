package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class AuthStatusShim(val statuses: Map<String, ProviderAuthStatus> = emptyMap()) {
    val antigravity: ProviderAuthStatus get() = statuses["antigravity"] ?: ProviderAuthStatus(false)
    val codex: ProviderAuthStatus get() = statuses["codex"] ?: ProviderAuthStatus(false)
    val devin: ProviderAuthStatus get() = statuses["devin"] ?: ProviderAuthStatus(false)
}
