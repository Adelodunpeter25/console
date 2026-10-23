package com.console.mobile.data.model

import com.console.mobile.SharedNative

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
 * Rejects anything it cannot read exactly, rather than repairing it. The
 * parser is the desktop's, shared over JNI (`console-shared`), so a paste that
 * one client accepts the other accepts too. When the shared library is not
 * loaded the paste is refused rather than parsed leniently: this is a
 * credential, and "unavailable" must not read as "valid".
 */
fun parsePairingUrl(raw: String): Result<PairingUrl> =
    SharedNative.pairing(raw).map { PairingUrl(baseUrl = it.baseUrl, serverId = it.serverId, code = it.code) }

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
 * before it is the address. The code lives in the fragment because a browser
 * never sends a fragment to the server, so it stays out of access logs.
 *
 * The code is empty when the text carries none, which is the plain "add a
 * server without pairing" case.
 */
fun splitHostAndCode(text: String): Pair<String, String> = SharedNative.splitPairing(text)

/** The host of a pairing URL, in the "host:port" form the host field takes. */
fun pairingHost(raw: String): String? =
    parsePairingUrl(raw).getOrNull()?.baseUrl?.let { SharedNative.displayHost(it) }

/** The pairing code of a pairing URL. */
fun pairingCode(raw: String): String? = parsePairingUrl(raw).getOrNull()?.code
