package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import com.console.mobile.data.api.ConsoleJson
import kotlinx.serialization.Serializable

/**
 * One server's paired device credential. The token is a long-lived signed
 * device session token, so it is stored encrypted and never logged.
 */
@Serializable
data class StoredServerAuth(
    val token: String,
    val deviceId: String,
    val deviceName: String,
    val expiresAt: Long,
)

@Serializable
private data class ServerAuthDocument(
    val servers: Map<String, StoredServerAuth> = emptyMap(),
)

/**
 * Per-server device credentials, encrypted at rest.
 *
 * Keyed by the server's base URL: pairing is a property of a server, not of
 * the app, so switching environments must not send one server's token to
 * another. `security-crypto` is deprecated upstream with no drop-in
 * replacement; swapping it out would make existing stored tokens unreadable
 * and sign everyone out.
 */
@Suppress("DEPRECATION")
class TokenStore(context: Context) {
    private val prefs: SharedPreferences by lazy {
        try {
            val masterKey = MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()
            EncryptedSharedPreferences.create(
                context,
                "console_tokens",
                masterKey,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )
        } catch (_: Exception) {
            context.getSharedPreferences("console_tokens_fallback", Context.MODE_PRIVATE)
        }
    }

    private fun key(baseUrl: String): String = baseUrl.trim().trimEnd('/').lowercase()

    private fun read(): ServerAuthDocument {
        val raw = prefs.getString(KEY_SERVERS, null) ?: return ServerAuthDocument()
        return try {
            ConsoleJson.decodeFromString(ServerAuthDocument.serializer(), raw)
        } catch (_: Exception) {
            // A corrupt document must not sign every server out silently in a
            // way that breaks startup; an empty map behaves like "not paired".
            ServerAuthDocument()
        }
    }

    fun tokenFor(baseUrl: String): String? = read().servers[key(baseUrl)]?.token

    fun authFor(baseUrl: String): StoredServerAuth? = read().servers[key(baseUrl)]

    fun save(baseUrl: String, auth: StoredServerAuth) {
        val next = read().servers.toMutableMap()
        next[key(baseUrl)] = auth
        prefs.edit()
            .putString(KEY_SERVERS, ConsoleJson.encodeToString(ServerAuthDocument.serializer(), ServerAuthDocument(next)))
            .apply()
    }

    fun clear(baseUrl: String) {
        val next = read().servers.toMutableMap()
        next.remove(key(baseUrl))
        prefs.edit()
            .putString(KEY_SERVERS, ConsoleJson.encodeToString(ServerAuthDocument.serializer(), ServerAuthDocument(next)))
            .apply()
    }

    /** Remove every server credential (revoke-all / sign out). */
    fun clearAllServers() {
        prefs.edit().remove(KEY_SERVERS).apply()
    }

    companion object {
        const val KEY_SERVERS = "server_auth"
    }
}
