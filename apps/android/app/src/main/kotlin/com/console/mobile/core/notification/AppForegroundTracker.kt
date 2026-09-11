package com.console.mobile.core.notification

import android.app.Activity
import android.app.Application
import android.os.Bundle
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * Foreground/background tracking (port of apps/mobile/utils/app-focus-manager.ts).
 * Phase 1 wires this to refetchOnWindowFocus / staleTime equivalent.
 */
object AppForegroundTracker {
    private val _isForeground = MutableStateFlow(false)
    val isForeground: StateFlow<Boolean> = _isForeground

    private var activityCount = 0

    fun register(app: Application) {
        app.registerActivityLifecycleCallbacks(object : Application.ActivityLifecycleCallbacks {
            override fun onActivityStarted(activity: Activity) {
                if (activityCount == 0) _isForeground.value = true
                activityCount++
            }
            override fun onActivityStopped(activity: Activity) {
                activityCount--
                if (activityCount == 0) _isForeground.value = false
            }
            override fun onActivityCreated(activity: Activity, savedInstanceState: Bundle?) {}
            override fun onActivityDestroyed(activity: Activity) {}
            override fun onActivityResumed(activity: Activity) {}
            override fun onActivityPaused(activity: Activity) {}
            override fun onActivitySaveInstanceState(activity: Activity, outState: Bundle) {}
        })
    }
}
