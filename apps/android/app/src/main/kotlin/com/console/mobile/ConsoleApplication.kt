package com.console.mobile

import android.app.Application
import com.console.mobile.core.notification.AppForegroundTracker

class ConsoleApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        AppForegroundTracker.register(this)
        AppContainer.initialize(this)
    }
}
