package com.console.mobile.core.util

/**
 * The scheme a scheme-less address speaks.
 *
 * A public endpoint — the tunnel address a QR names, or any reverse proxy —
 * only answers https, and asking it over http reads as "not found" rather
 * than as a fixable error. A scheme-less address stays http only when it is
 * plausibly local: it carries an explicit port, is an IP or localhost, or is
 * a bare machine name on the LAN.
 */
private fun defaultScheme(address: String): String {
    val authority = address.substringBefore('/').trimEnd('/')
    val hostPort = authority.substringAfterLast('@')
    val hostNoBrackets = if (hostPort.startsWith("[")) {
        hostPort.substringAfter('[').substringBefore(']')
    } else {
        hostPort.substringBefore(':')
    }
    val explicitPort = hostPort.substringAfter(':', "").takeIf { it.isNotEmpty() && it.all(Char::isDigit) }
    if (explicitPort != null) return "http"
    if (hostNoBrackets == "localhost" || hostNoBrackets.endsWith(".localhost")) return "http"
    // The address is an IP literal when every segment is numeric (or it is a
    // colon-separated v6 form); a dotted name is a public endpoint, and a
    // bare name is a machine on the LAN.
    val isIp = hostNoBrackets.matches(Regex("[0-9a-fA-F:.]+")) &&
        (hostNoBrackets.contains(':') || hostNoBrackets.count { it == '.' } == 3)
    return if (isIp || !hostNoBrackets.contains('.')) "http" else "https"
}

/** The scheme of an address reached over iroh; its host is a 64-hex node ID. */
const val IROH_SCHEME = "console-iroh://"

private val NODE_ID = Regex("[0-9a-f]{64}")

/** The node ID of a `console-iroh://<node-id>[/...]` address, or null. */
fun irohNodeId(url: String): String? =
    url.trim().takeIf { it.startsWith(IROH_SCHEME) }
        ?.removePrefix(IROH_SCHEME)?.substringBefore('/')?.substringBefore('#')
        ?.takeIf { NODE_ID.matches(it) }

/**
 * The URL requests to [serverUrl] are sent to: an iroh address becomes the
 * loopback port of its tunnel, anything else is returned unchanged. When the
 * tunnel cannot be made the address is returned as is, so the request fails
 * with a connection error instead of reaching the wrong server.
 */
fun reachableUrl(serverUrl: String): String {
    val nodeId = irohNodeId(serverUrl) ?: return serverUrl
    return com.console.mobile.SharedNative.irohTunnel(nodeId) ?: serverUrl
}

fun normalizeBackendUrl(input: String): String? {
    var url = input.trim().trimEnd('/')
    if (url.isEmpty()) return null
    if (url.startsWith(IROH_SCHEME)) return url.takeIf { irohNodeId(it) != null }
    if (!url.startsWith("http://") && !url.startsWith("https://")) url = "${defaultScheme(url)}://$url"
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
