package com.console.mobile.data.model

import kotlinx.serialization.Serializable

object Providers {
    const val ANTIGRAVITY = "antigravity"
    const val OPENCODE = "opencode"
    const val CODEX = "codex"
    const val CLINE = "cline"
    const val DEVIN = "devin"
}

object ThinkingLevels {
    const val NONE = "none"
    const val MINIMAL = "minimal"
    const val LOW = "low"
    const val MEDIUM = "medium"
    const val HIGH = "high"
    const val XHIGH = "xhigh"
    const val MAX = "max"
}

@Serializable
data class Model(
    val id: String,
    val provider: String,
    val contextWindow: Int,
    val supportsImages: Boolean = false,
    val supportedThinkingLevels: List<String> = emptyList(),
    val defaultThinkingLevel: String? = null,
)

@Serializable
data class ModelFavorite(
    val provider: String,
    val modelId: String,
)

@Serializable
data class ProviderCatalogEntry(
    val name: String,
    val displayName: String,
    val description: String,
    val models: List<Model> = emptyList(),
    val authMethod: String,
)
