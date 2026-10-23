package com.console.mobile.core.util

import com.console.mobile.SharedNative

fun folderName(path: String?): String {
    if (path.isNullOrEmpty()) return ""
    val trimmed = path.trimEnd('/')
    val i = trimmed.lastIndexOf('/')
    return if (i != -1) trimmed.substring(i + 1) else trimmed
}

/** Human-size context window, from the desktop's formatter: 1_000_000 -> "1M", 272_000 -> "272k". */
fun formatContextWindow(contextWindow: Int): String =
    SharedNative.contextWindow(contextWindow) ?: contextWindow.toString()

fun formatModelName(modelId: String?): String {
    if (modelId.isNullOrEmpty()) return ""
    return SharedNative.modelName(modelId) ?: modelId
}

fun formatDurationMs(ms: Long): String {
    val seconds = maxOf(0L, Math.round(ms / 1000.0))
    if (seconds < 60) return "${seconds}s"
    val minutes = seconds / 60
    return "$minutes m ${(seconds % 60).toString().padStart(2, '0')}s".replace(" ", "")
        .let { "${minutes}m ${(seconds % 60).toString().padStart(2, '0')}s" }
}

/** Footer label for the server-computed output rate, e.g. "42.1 tok/s".
 *  The Go server owns the measurement and its gating; null hides the metric. */
fun formatTokensPerSecond(tokPerSec: Double?): String? = SharedNative.tokensPerSecond(tokPerSec)
