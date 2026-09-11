package com.console.mobile.core.notification

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build

object LocalNotificationPresenter {
    const val CHANNEL_ID = "console_general"
    const val EXTRA_THREAD_ID = "extra_thread_id"

    fun ensureChannelCreated(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (nm.getNotificationChannel(CHANNEL_ID) != null) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            "Console",
            NotificationManager.IMPORTANCE_DEFAULT,
        )
        nm.createNotificationChannel(channel)
    }

    fun consumeLaunchToken(context: Context, intent: android.content.Intent?, threadId: String): Boolean {
        // Phase 0: just validate token ownership; Phase 1 adds real consume logic.
        return threadId.isNotEmpty()
    }
}
