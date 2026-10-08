package com.console.mobile.data.model

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

// Model / ProviderCatalogEntry moved to the shared protobuf schema
// (console.v1 from proto/console/v1/catalog.proto): contextWindow stays a
// JSON number, thinking levels stay plain strings, and an omitted model
// list decodes to empty instead of null.

/** Star key format, matching the desktop's `"{provider}:{model_id}"`. */
fun favoriteKey(provider: String, modelId: String): String = "$provider:$modelId"
