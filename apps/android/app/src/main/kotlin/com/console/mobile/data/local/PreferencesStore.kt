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

    /**
     * Environment ids whose setup panel was already dismissed.
     *
     * One bit per server, keyed by environment id — the same ids
     * [PersistedEnvironments] uses. Empty means every server still shows the
     * panel, which is what makes a fresh install and an upgraded one behave the
     * same (nothing stored reads as "nothing dismissed").
     *
     * Deliberately holds no setup state. Whether a server has a project or a
     * provider key is read live; storing it here would let the two disagree the
     * moment a user deleted their last project.
     */
    fun loadOnboardingDismissed(): Set<String> =
        prefs.getStringSet(KEY_ONBOARDING_DISMISSED, emptySet())?.toSet() ?: emptySet()

    /**
     * Mark one server's setup panel dismissed, leaving every other server's bit
     * alone.
     *
     * A `StringSet` is copied before writing because Android hands back the live
     * stored instance — mutating it in place updates memory only, and the change
     * is silently lost.
     */
    fun dismissOnboarding(envId: String) {
        val next = loadOnboardingDismissed().toMutableSet()
        next.add(envId)
        prefs.edit().putStringSet(KEY_ONBOARDING_DISMISSED, next).apply()
    }

    companion object {
        const val KEY_BACKEND_URL = "backend_url"
        const val KEY_ENVIRONMENTS = "environments_data"
        const val KEY_ONBOARDING_DISMISSED = "onboarding_dismissed"
    }
}
