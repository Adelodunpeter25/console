package com.console.mobile.core.util

/** Port of apps/mobile/utils/format.ts */
fun folderName(path: String?): String {
    if (path.isNullOrEmpty()) return ""
    val trimmed = path.trimEnd('/')
    val i = trimmed.lastIndexOf('/')
    return if (i != -1) trimmed.substring(i + 1) else trimmed
}

fun formatModelName(modelId: String?): String {
    if (modelId.isNullOrEmpty()) return ""
    return modelId.split('-', '_').joinToString(" ") { part ->
        if (part.isNotEmpty()) part[0].uppercaseChar() + part.substring(1) else part
    }
}

fun formatDurationMs(ms: Long): String {
    val seconds = maxOf(0L, Math.round(ms / 1000.0))
    if (seconds < 60) return "${seconds}s"
    val minutes = seconds / 60
    return "$minutes m ${(seconds % 60).toString().padStart(2, '0')}s".replace(" ", "")
        .let { "${minutes}m ${(seconds % 60).toString().padStart(2, '0')}s" }
}
