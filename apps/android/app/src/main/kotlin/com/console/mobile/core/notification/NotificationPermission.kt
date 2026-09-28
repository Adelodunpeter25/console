package com.console.mobile.core.notification

import android.Manifest
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.platform.LocalContext
import androidx.core.app.NotificationManagerCompat

/**
 * Asks for POST_NOTIFICATIONS on API 33+ so [LocalNotificationPresenter.showNotification]
 * actually posts. Without the runtime grant, `notify()` is a silent no-op and the
 * whole "run finished → notify → tap to open the chat" flow never fires.
 *
 * Called from the nav graph only once a backend is connected, so we don't ask for a
 * permission before the app can do anything useful. Deliberately not naggy: if the
 * user has denied twice Android stops showing the dialog, and `launch` becomes a
 * no-op rather than throwing.
 */
@Composable
fun NotificationPermissionRequest() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return

    val context = LocalContext.current
    val launcher = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestPermission(),
    ) { /* granted or denied — notifications simply don't post if denied */ }

    LaunchedEffect(Unit) {
        if (!NotificationManagerCompat.from(context).areNotificationsEnabled()) {
            launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}
