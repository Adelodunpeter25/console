package com.console.mobile.data.local

import android.content.Context
import android.content.SharedPreferences
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.store.Environment
import kotlinx.serialization.Serializable

@Serializable
data class PersistedEnvironments(
    val environments: List<Environment> = emptyList(),
    val activeId: String? = null,
)

class PreferencesStore(context: Context) {
    private val prefs: SharedPreferences =
        context.getSharedPreferences("console_prefs", Context.MODE_PRIVATE)

    var backendUrl: String?
        get() = prefs.getString(KEY_BACKEND_URL, null)
        set(value) {
            if (value != null) prefs.edit().putString(KEY_BACKEND_URL, value).apply()
            else prefs.edit().remove(KEY_BACKEND_URL).apply()
        }

    fun loadEnvironments(): Pair<List<Environment>, String?> {
        val raw = prefs.getString(KEY_ENVIRONMENTS, null)
        if (raw != null) {
            try {
                val parsed = ConsoleJson.decodeFromString(PersistedEnvironments.serializer(), raw)
                val envs = parsed.environments
                val activeId = if (parsed.activeId != null && envs.any { it.id == parsed.activeId }) parsed.activeId else null
                return envs to activeId
            } catch (_: Exception) {
            }
        }
        return emptyList<Environment>() to null
    }

    fun saveEnvironments(environments: List<Environment>, activeId: String?) {
        val activeUrl = environments.firstOrNull { it.id == activeId }?.url
        backendUrl = activeUrl
        val payload = PersistedEnvironments(environments = environments, activeId = activeId)
        val json = ConsoleJson.encodeToString(PersistedEnvironments.serializer(), payload)
        prefs.edit().putString(KEY_ENVIRONMENTS, json).apply()
    }

    fun clearAll() {
        prefs.edit().clear().apply()
    }

    companion object {
        const val KEY_BACKEND_URL = "backend_url"
        const val KEY_ENVIRONMENTS = "environments_data"
    }
}
