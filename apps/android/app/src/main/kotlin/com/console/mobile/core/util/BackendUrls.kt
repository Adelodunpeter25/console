package com.console.mobile.core.util

/** Port of apps/mobile/utils/url.ts */
fun normalizeBackendUrl(input: String): String? {
    var url = input.trim().trimEnd('/')
    if (url.isEmpty()) return null
    if (!url.startsWith("http://") && !url.startsWith("https://")) url = "http://$url"
    return url
}

fun urlHost(url: String): String = try {
    java.net.URI(url).host ?: url
} catch (_: Exception) {
    url
}
