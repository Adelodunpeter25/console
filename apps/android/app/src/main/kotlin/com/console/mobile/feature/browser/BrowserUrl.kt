package com.console.mobile.feature.browser

/** Schemes the in-app browser loads itself; every other scheme is handed off. */
private val WEB_SCHEMES = setOf("http", "https", "file", "about", "data", "blob", "javascript")

/** host[:port][/path] — a bare address, no scheme, no spaces. */
private val ADDRESS_LIKE = Regex("^[A-Za-z0-9._~%-]+(:\\d{1,5})?(/.*)?$")

/** True when the WebView can render this URL itself (relative URLs count). */
fun isWebUrl(url: String): Boolean {
    val scheme = url.substringBefore(':', "").lowercase()
    return scheme.isEmpty() || scheme in WEB_SCHEMES
}

/**
 * Turns address-bar text into something worth loading, or null when it is not
 * an address at all.
 *
 * A scheme-less address is read as a dev server — `localhost:5173`,
 * `10.0.2.2:3000`, `192.168.1.20` — and gets `http://`, because that is what
 * forwarded ports speak. Anything already carrying a scheme is passed through
 * untouched, so `https://…` and `about:blank` still work.
 */
fun normalizeBrowserUrl(input: String): String? {
    val trimmed = input.trim()
    if (trimmed.isEmpty()) return null
    if (trimmed.contains("://") || trimmed.startsWith("about:") || trimmed.startsWith("data:")) {
        return trimmed
    }
    if (!looksLikeAddress(trimmed)) return null
    return "http://$trimmed"
}

private fun looksLikeAddress(input: String): Boolean {
    if (input.any { it.isWhitespace() }) return false
    if (!ADDRESS_LIKE.matches(input)) return false
    val host = input.substringBefore('/').substringBefore(':')
    return host.contains('.') || host.equals("localhost", ignoreCase = true)
}

/** True for pages worth a lock rather than a "not secure" globe. */
fun isSecureUrl(url: String): Boolean = when (url.substringBefore(':', "").lowercase()) {
    "https", "file", "about", "data" -> true
    else -> false
}
