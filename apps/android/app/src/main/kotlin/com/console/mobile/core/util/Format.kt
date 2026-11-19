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
    val secs = (seconds % 60).toString().padStart(2, '0')
    return "${minutes}m ${secs}s"
}

/** Footer label for the server-computed output rate, e.g. "42.1 tok/s".
 *  The Go server owns the measurement and its gating; null hides the metric. */
fun formatTokensPerSecond(tokPerSec: Double?): String? = SharedNative.tokensPerSecond(tokPerSec)

/** Simplify a CSS selector for display in element annotation chips. */
fun formatSelectorLabel(selector: String): String {
    SharedNative.selectorLabel(selector)?.let { return it }
    val trimmed = selector.trim()
    if (trimmed.isEmpty()) return ""
    val leaf = trimmed.substringAfterLast('>').substringAfterLast('+').substringAfterLast('~').substringAfterLast(' ').trim()
    if (leaf.startsWith('#')) return leaf
    val base = leaf.substringBefore(':')
    val pseudo = if (':' in leaf) ":" + leaf.substringAfter(':') else ""
    val parts = base.split('.').filter { it.isNotEmpty() }
    val out = StringBuilder()
    if (!base.startsWith('.')) {
        if (parts.isNotEmpty()) out.append(parts[0])
        if (parts.size > 1) out.append('.').append(parts[1])
    } else {
        if (parts.isNotEmpty()) out.append('.').append(parts[0])
    }
    out.append(pseudo)
    return if (out.isEmpty()) leaf else out.toString()
}
