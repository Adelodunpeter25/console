package com.console.mobile.feature.settings

import android.content.Context
import android.net.Uri
import androidx.browser.customtabs.CustomTabsIntent
import com.console.mobile.data.repo.AuthRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * OAuth login via Custom Tab: GET login URL → open browser → deep-link callback
 * handled by OAuthDeepLinkHandler → submitCallback → loadStatus.
 * Mirrors useLocalOAuthLogin / useOAuthDeepLink in Expo (local-auth-server → Custom Tab).
 */
object OAuthLoginLauncher {
    suspend fun login(context: Context, authRepo: AuthRepository, provider: String) {
        val result = authRepo.getLoginUrl(provider)
        OAuthDeepLinkHandler.pendingState = result.state
        OAuthDeepLinkHandler.pendingProvider = provider
        withContext(Dispatchers.Main) {
            val intent = CustomTabsIntent.Builder().setShowTitle(true).build()
            intent.launchUrl(context, Uri.parse(result.authUrl))
        }
    }
}

/** Deep-link target state — MainActivity routes VIEW intents here. */
object OAuthDeepLinkHandler {
    var pendingState: String? = null
    var pendingProvider: String? = null

    /** Returns true if the intent was an OAuth callback we consumed. */
    fun handleDeepLink(uri: Uri?, onResult: (Result<Unit>) -> Unit): Boolean {
        val u = uri ?: return false
        val code = u.getQueryParameter("code") ?: return false
        val provider = pendingProvider ?: u.getQueryParameter("provider") ?: return false
        val state = u.getQueryParameter("state")
        pendingProvider = null
        pendingState = null
        // Caller runs submitCallback on IO.
        OAuthCallbackBus.pending = OAuthCallbackBus.Pending(provider, code, state)
        onResult(Result.success(Unit))
        return true
    }
}

object OAuthCallbackBus {
    data class Pending(val provider: String, val code: String, val state: String?)
    @Volatile var pending: Pending? = null
}
