package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

// security-crypto is deprecated upstream with no drop-in replacement; swapping it out
// would make existing stored tokens unreadable and sign everyone out.
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

    var authToken: String?
        get() = prefs.getString(KEY_TOKEN, null)
        set(value) {
            if (value == null) prefs.edit().remove(KEY_TOKEN).apply()
            else prefs.edit().putString(KEY_TOKEN, value).apply()
        }

    var deviceId: String?
        get() = prefs.getString(KEY_DEVICE_ID, null)
        set(value) {
            if (value == null) prefs.edit().remove(KEY_DEVICE_ID).apply()
            else prefs.edit().putString(KEY_DEVICE_ID, value).apply()
        }

    var deviceName: String?
        get() = prefs.getString(KEY_DEVICE_NAME, null)
        set(value) {
            if (value == null) prefs.edit().remove(KEY_DEVICE_NAME).apply()
            else prefs.edit().putString(KEY_DEVICE_NAME, value).apply()
        }

    var tokenExpiresAt: Long
        get() = prefs.getLong(KEY_TOKEN_EXPIRES_AT, 0L)
        set(value) { prefs.edit().putLong(KEY_TOKEN_EXPIRES_AT, value).apply() }

    companion object {
        const val KEY_TOKEN = "auth_token"
        const val KEY_DEVICE_ID = "device_id"
        const val KEY_DEVICE_NAME = "device_name"
        const val KEY_TOKEN_EXPIRES_AT = "auth_token_expires_at"
    }
}
