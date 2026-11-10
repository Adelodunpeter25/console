package com.console.mobile

import android.app.Application
import com.console.mobile.core.notification.AppForegroundTracker

import androidx.compose.foundation.ComposeFoundationFlags

class ConsoleApplication : Application() {
    override fun onCreate() {
        // Compose 1.10.x enables a new context menu pipeline by default that
        // bypasses LocalTextToolbar. Keep the classic toolbar pipeline so our
        // custom floating Reply/Copy SelectionActionToolbar gets invoked.
        ComposeFoundationFlags.isNewContextMenuEnabled = false
        super.onCreate()
        AppForegroundTracker.register(this)
        AppContainer.initialize(this)
    }
}
