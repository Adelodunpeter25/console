package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences

/**
 * Phase 0 stub. Phase 1 replaces with DataStore Preferences / EncryptedSharedPreferences
 * for backendUrl + environments + favorites (mirrors react-native-mmkv + utils/storage.ts).
 * Kept as SharedPreferences for now so Phase 0 builds without extra deps.
 */
class PreferencesStore(context: Context) {
    private val prefs: SharedPreferences =
        context.getSharedPreferences("console_prefs", Context.MODE_PRIVATE)

    var backendUrl: String?
        get() = prefs.getString(KEY_BACKEND_URL, null)
        set(value) { prefs.edit().putString(KEY_BACKEND_URL, value).apply() }

    companion object {
        const val KEY_BACKEND_URL = "backend_url"
    }
}
