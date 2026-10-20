package com.console.mobile.data.model

/**
 * The pairing URL: the one string a user copies and pastes to connect.
 *
 * Shape (produced by `console pair` / `console status`):
 * ```
 * http://server.example:3000/console/pair#v=1&server=<server-id>&grant=<one-time-grant>
 * ```
 *
 * The secret lives in the fragment, which a browser never sends to the
 * server, so the grant does not end up in access logs. The base URL is part
 * of the same string, so pasting it is enough: the app learns both which
 * server to talk to and how to prove it may.
 *
 * The parser is strict on purpose. The grant is a credential, and a lenient
 * parser is how a malformed paste becomes a pairing with the wrong machine.
 */
data class PairingUrl(
    /** The server's base URL: scheme, host and port. */
    val baseUrl: String,
    /** The server the URL was minted by. */
    val serverId: String,
    /** The one-time pairing grant. */
    val grant: String,
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
    if (schemeEnd <= 0) {
        return Result.failure(IllegalArgumentException("That is not a pairing URL — it has no address."))
    }
    val scheme = base.substring(0, schemeEnd)
    if (scheme != "http" && scheme != "https") {
        return Result.failure(IllegalArgumentException("A pairing URL must start with http:// or https://."))
    }
    val authority = base.substring(schemeEnd + 3).substringBefore('/')
    if (authority.isBlank()) {
        return Result.failure(IllegalArgumentException("A pairing URL must name a server."))
    }

    var version = ""
    var serverId = ""
    var grant = ""
    for (pair in fragment.split('&')) {
        val equals = pair.indexOf('=')
        if (equals < 0) continue
        val key = pair.substring(0, equals)
        val value = percentDecode(pair.substring(equals + 1))
        when (key) {
            "v" -> version = value
            "server" -> serverId = value
            "grant" -> grant = value
        }
    }
    if (version != "1") {
        return Result.failure(IllegalArgumentException("That pairing URL was made by a newer version of Console."))
    }
    if (serverId.isEmpty()) {
        return Result.failure(IllegalArgumentException("That pairing URL does not name a server."))
    }
    if (grant.isEmpty()) {
        return Result.failure(IllegalArgumentException("That pairing URL has no pairing code."))
    }
    return Result.success(PairingUrl(baseUrl = "$scheme://$authority", serverId = serverId, grant = grant))
}

/**
 * The server's base URL from a pairing URL, or null when the input is not
 * a pairing URL at all.
 */
fun pairingBaseUrl(raw: String): String? = parsePairingUrl(raw).getOrNull()?.baseUrl

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
