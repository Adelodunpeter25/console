package com.console.mobile.data.model

/**
 * The pairing details a user copies off the server.
 *
 * The server prints two things: a host ("192.168.1.100:8080") and a short
 * pairing code ("abc-123-xyz"). They can be typed into the two fields of
 * the Add Environment sheet, or the camera can read them off a QR code.
 *
 * A QR code carries the whole pairing URL:
 *
 * ```
 * http://192.168.1.100:8080/console/pair#v=1&server=481e2796&code=abc-123-xyz
 * ```
 *
 * The code is in the fragment, which a browser never sends to the server, so
 * it does not end up in access logs.
 *
 * The parser is strict on purpose. The code is a credential, and a lenient
 * parser is how a malformed paste becomes a pairing with the wrong machine.
 */
data class PairingUrl(
    /** The server's base URL: scheme, host and port. */
    val baseUrl: String,
    /** The server the URL was minted by. */
    val serverId: String,
    /** The one-time pairing code. */
    val code: String,
)

/**
 * Parse a pairing URL into its parts.
 *
 * Rejects anything it cannot read exactly, rather than repairing it.
 */
fun parsePairingUrl(raw: String): Result<PairingUrl> {
    val trimmed = raw.trim()
    if (trimmed.isEmpty()) {
        return Result.failure(IllegalArgumentException("The pairing URL is empty."))
    }
    val hashIndex = trimmed.indexOf('#')
    if (hashIndex < 0) {
        return Result.failure(IllegalArgumentException("That is not a pairing URL — it has no pairing code."))
    }
    val base = trimmed.substring(0, hashIndex)
    val fragment = trimmed.substring(hashIndex + 1)

    val schemeEnd = base.indexOf("://")
    val scheme: String
    val authority: String
    if (schemeEnd <= 0) {
        // No scheme: an address on its own. The scheme is only assumed when
        // the text has none at all, so "192.168.1.10:3000" and
        // "http://192.168.1.10:3000" both work.
        scheme = "http"
        authority = base
    } else {
        scheme = base.substring(0, schemeEnd)
        if (scheme != "http" && scheme != "https") {
            return Result.failure(IllegalArgumentException("A pairing URL must start with http:// or https://."))
        }
        authority = base.substring(schemeEnd + 3)
    }
    // The path is /console/pair; the address is just the authority.
    val host = authority.substringBefore('/')
    if (host.isBlank()) {
        return Result.failure(IllegalArgumentException("A pairing URL must name a server."))
    }

    var version = ""
    var serverId = ""
    var code = ""
    var query = false
    for (pair in fragment.split('&')) {
        val equals = pair.indexOf('=')
        if (equals <= 0) continue
        query = true
        val key = pair.substring(0, equals)
        val value = percentDecode(pair.substring(equals + 1))
        when (key) {
            "v" -> version = value
            "server" -> serverId = value
            "code" -> code = value
        }
    }
    if (query) {
        // The full URL carries a version, so an unknown one is refused
        // rather than guessed at.
        if (version != "1") {
            return Result.failure(IllegalArgumentException("That pairing URL was made by a newer version of Console."))
        }
        if (code.isEmpty()) {
            return Result.failure(IllegalArgumentException("That pairing URL has no pairing code."))
        }
    } else {
        // A bare fragment is the code itself.
        code = fragment
        if (code.isEmpty()) {
            return Result.failure(IllegalArgumentException("That pairing URL has no pairing code."))
        }
    }
    return Result.success(PairingUrl(baseUrl = "$scheme://$host", serverId = serverId, code = code))
}

/**
 * Split pasted text into an address and a pairing code.
 *
 * Three shapes are accepted, and they all mean the same thing:
 *
 *  - "192.168.1.100:8080" — an address, no code
 *  - "192.168.1.100:8080#abc-123-xyz" — an address and a code
 *  - "http://192.168.1.100:8080/console/pair#v=1&server=..&code=abc-123-xyz" — the whole URL
 *
 * The rule is one line: everything after the "#" is the code, everything
 * before it is the address. The code lives in the fragment because a
 * browser never sends a fragment to the server, so it stays out of access
 * logs.
 *
 * The code is empty when the text carries none, which is the plain
 * "add a server without pairing" case.
 */
fun splitHostAndCode(text: String): Pair<String, String> {
    val trimmed = text.trim()
    if (trimmed.isEmpty()) return "" to ""

    val hash = trimmed.indexOf('#')
    if (hash < 0) return trimmed to ""

    val base = trimmed.substring(0, hash)
    val fragment = trimmed.substring(hash + 1)

    // A bare code, or a query string the full URL carries.
    if (!fragment.contains('=')) return base to fragment
    val params = fragment.split('&').mapNotNull { pair ->
        val equals = pair.indexOf('=')
        if (equals <= 0) null else pair.substring(0, equals) to pair.substring(equals + 1)
    }.toMap()
    val code = params["code"].orEmpty()
    if (code.isEmpty()) return base to ""
    // The full URL carries a version, so an unknown one is refused rather
    // than guessed at.
    if (params["v"] != null && params["v"] != "1") return base to ""
    // The URL carries a path; the address is just the authority.
    val authority = base.substringAfter("://", base).substringBefore('/')
    return authority to code
}

/**
 * The host of a pairing URL, in the "host:port" form the host field takes.
 */
fun pairingHost(raw: String): String? =
    parsePairingUrl(raw).getOrNull()?.baseUrl?.let { urlHostPortOf(it) }

/**
 * The pairing code of a pairing URL.
 */
fun pairingCode(raw: String): String? = parsePairingUrl(raw).getOrNull()?.code

/**
 * "192.168.1.102:2003" — the host plus its port, since two servers on the
 * same LAN are told apart by the port rather than the host.
 */
private fun urlHostPortOf(url: String): String = try {
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

/**
 * Percent-decode a fragment value.
 *
 * Only the escapes that can appear in a URL-encoded query value are
 * handled; anything else is left as-is rather than guessed at.
 */
private fun percentDecode(value: String): String {
    if (!value.contains('%') && !value.contains('+')) return value
    val bytes = java.io.ByteArrayOutputStream(value.length)
    var i = 0
    while (i < value.length) {
        when (val c = value[i]) {
            '%' -> {
                val hex = if (i + 2 < value.length) value.substring(i + 1, i + 3) else ""
                val decoded = hex.toIntOrNull(16)
                if (hex.length == 2 && decoded != null) {
                    bytes.write(decoded)
                    i += 3
                } else {
                    bytes.write(c.code)
                    i += 1
                }
            }
            '+' -> {
                bytes.write(' '.code)
                i += 1
            }
            else -> {
                bytes.write(c.toString().toByteArray(Charsets.UTF_8))
                i += 1
            }
        }
    }
    return bytes.toString(Charsets.UTF_8.name())
}
