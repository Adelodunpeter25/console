package com.console.mobile.core.util

fun normalizeBackendUrl(input: String): String? {
    var url = input.trim().trimEnd('/')
    if (url.isEmpty()) return null
    if (!url.startsWith("http://") && !url.startsWith("https://")) url = "http://$url"
    return url
}

/**
 * "192.168.1.102:2003" — the host plus its port when the URL carries one, since
 * two servers on the same LAN are told apart by the port, not the host. Falls
 * back to the bare host, then to the raw string if the URL won't parse.
 */
fun urlHostPort(url: String): String = try {
    val uri = java.net.URI(url)
    val host = uri.host
    val port = uri.port
    when {
        host == null -> url
        port > 0 -> "$host:$port"
        else -> host
    }
} catch (_: Exception) {
    url
}
