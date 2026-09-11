package com.console.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class AuthStatusShim(
    val antigravity: ProviderAuthStatus = ProviderAuthStatus(false),
    val codex: ProviderAuthStatus = ProviderAuthStatus(false),
    val devin: ProviderAuthStatus = ProviderAuthStatus(false),
)
