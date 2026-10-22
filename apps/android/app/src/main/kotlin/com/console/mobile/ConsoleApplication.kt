package com.console.mobile

import android.app.Application
import com.console.mobile.core.notification.AppForegroundTracker
import ru.noties.jlatexmath.JLatexMathAndroid

class ConsoleApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        AppForegroundTracker.register(this)
        JLatexMathAndroid.init(this)
        AppContainer.initialize(this)
    }
}
