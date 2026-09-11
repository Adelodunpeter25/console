package com.console.mobile

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import com.console.mobile.core.notification.LocalNotificationPresenter
import com.console.mobile.ui.navigation.AppNavGraph
import com.console.mobile.ui.theme.ConsoleTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        handleNotificationLaunchIntent(intent)
        setContent {
            val context = LocalContext.current
            val configuration = LocalConfiguration.current
            // Theme is dark-only (Console #0a0a0b) but we keep the hook so Phase 2
            // settings can toggle if needed. Mirrors Remodex MainActivity structure.
            val systemDark = isSystemInDarkTheme()
            val darkTheme = remember(systemDark) { true }

            ConsoleTheme(darkTheme = darkTheme) {
                AppNavGraph()
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleNotificationLaunchIntent(intent)
    }

    private fun handleNotificationLaunchIntent(intent: Intent?) {
        val tid = intent?.getStringExtra(LocalNotificationPresenter.EXTRA_THREAD_ID)?.trim() ?: return
        if (tid.isNotEmpty() && LocalNotificationPresenter.consumeLaunchToken(this, intent, tid)) {
            AppContainer.setPendingOpenFromNotification(tid)
        }
    }
}
