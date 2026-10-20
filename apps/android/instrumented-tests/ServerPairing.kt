package com.console.mobile

import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL

/**
 * Instrumentation-test helper for server device authentication.
 *
 * The server no longer mints a token from a caller-supplied device id: a
 * device token is issued only by exchanging a pairing grant. Start the dev
 * server without any paired device and pass the grant it prints on stderr as
 * `WT_SPIKE_PAIR_GRANT`.
 */
internal object ServerPairing {
    /**
     * Exchange [grant] for a device token, or null when the server is
     * unreachable or the grant was rejected.
     */
    fun pair(apiUrl: String, grant: String?): String? {
        if (grant.isNullOrBlank()) return null
        return try {
            val conn = URL("$apiUrl/api/auth/pair").openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.connectTimeout = 5_000
            conn.readTimeout = 5_000
            conn.setRequestProperty("Content-Type", "application/json")
            conn.outputStream.use {
                it.write(
                    """{"grant":"$grant","deviceName":"WT spike","platform":"android"}"""
                        .toByteArray(),
                )
            }
            val body = BufferedReader(InputStreamReader(conn.inputStream)).readText()
            Regex("\"token\"\\s*:\\s*\"([^\"]+)\"").find(body)?.groupValues?.get(1)
        } catch (_: Exception) {
            null
        }
    }
}
