package com.console.mobile.core.update

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.console.mobile.AppContainer
import com.console.mobile.MainActivity
import com.console.mobile.R
import com.console.mobile.core.notification.AppForegroundTracker
import com.console.mobile.core.notification.LocalNotificationPresenter
import java.io.File
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

sealed interface UpdateState {
    data object Idle : UpdateState
    data object Checking : UpdateState
    data object UpToDate : UpdateState
    data class Available(val manifest: UpdateManifest) : UpdateState
    data class Downloading(val manifest: UpdateManifest, val progress: Float) : UpdateState
    data class ReadyToInstall(val manifest: UpdateManifest, val apk: File) : UpdateState
    data object AllowInstalls : UpdateState
    data class Failed(val message: String) : UpdateState
}

object AppUpdateManager {
    const val UPDATE_NOTIFICATION_ID = 8888
    const val EXTRA_INSTALL_UPDATE = "extra_install_update"

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private val _state = MutableStateFlow<UpdateState>(UpdateState.Idle)
    val state: StateFlow<UpdateState> = _state.asStateFlow()

    @Volatile
    private var pendingInstallApk: File? = null

    fun checkForUpdate() {
        if (_state.value is UpdateState.Checking || _state.value is UpdateState.Downloading) return
        _state.value = UpdateState.Checking
        scope.launch {
            _state.value = try {
                val manifest = withContext(Dispatchers.IO) { AppUpdater.checkForUpdate() }
                if (manifest == null) UpdateState.UpToDate else UpdateState.Available(manifest)
            } catch (e: Exception) {
                UpdateState.Failed(e.message ?: "Unable to check for updates.")
            }
        }
    }

    fun downloadAndInstall(manifest: UpdateManifest) {
        if (_state.value is UpdateState.Downloading) return
        _state.value = UpdateState.Downloading(manifest, 0f)
        scope.launch {
            val context = AppContainer.appContext
            try {
                var lastNotificationTime = 0L
                showDownloadProgressNotification(context, manifest, 0f)
                val apk = withContext(Dispatchers.IO) {
                    AppUpdater.download { progress ->
                        _state.value = UpdateState.Downloading(manifest, progress)
                        val now = System.currentTimeMillis()
                        if (now - lastNotificationTime >= 500L || progress >= 1f) {
                            lastNotificationTime = now
                            showDownloadProgressNotification(context, manifest, progress)
                        }
                    }
                }
                _state.value = UpdateState.ReadyToInstall(manifest, apk)

                if (AppForegroundTracker.isForeground.value) {
                    cancelUpdateNotification(context)
                    val launched = AppUpdater.install(apk)
                    if (!launched) {
                        pendingInstallApk = apk
                        _state.value = UpdateState.AllowInstalls
                    }
                } else {
                    pendingInstallApk = apk
                    showDownloadCompleteNotification(context, manifest)
                }
            } catch (e: Exception) {
                cancelUpdateNotification(context)
                _state.value = UpdateState.Failed(e.message ?: "Unable to download the update.")
            }
        }
    }

    fun installPendingUpdate(): Boolean {
        val apk = when (val current = _state.value) {
            is UpdateState.ReadyToInstall -> current.apk
            else -> pendingInstallApk ?: File(File(AppContainer.appContext.cacheDir, "updates"), "app-release.apk")
        }
        if (!apk.exists()) return false

        cancelUpdateNotification(AppContainer.appContext)
        val success = AppUpdater.install(apk)
        if (!success) {
            pendingInstallApk = apk
            _state.value = UpdateState.AllowInstalls
        } else {
            pendingInstallApk = null
        }
        return success
    }

    fun onAppForegrounded() {
        val context = AppContainer.appContext
        val apk = pendingInstallApk
        if (apk != null && apk.exists()) {
            pendingInstallApk = null
            cancelUpdateNotification(context)
            val success = AppUpdater.install(apk)
            if (!success) {
                pendingInstallApk = apk
                _state.value = UpdateState.AllowInstalls
            }
            return
        }

        if (_state.value is UpdateState.AllowInstalls && context.packageManager.canRequestPackageInstalls()) {
            val cached = File(File(context.cacheDir, "updates"), "app-release.apk")
            if (cached.exists()) {
                AppUpdater.install(cached)
            }
        }
    }

    fun handleLaunchIntent(intent: Intent?): Boolean {
        val isUpdateLaunch = intent?.getBooleanExtra(EXTRA_INSTALL_UPDATE, false) ?: return false
        intent.removeExtra(EXTRA_INSTALL_UPDATE)
        cancelUpdateNotification(AppContainer.appContext)
        return installPendingUpdate()
    }

    private fun showDownloadProgressNotification(context: Context, manifest: UpdateManifest, progress: Float) {
        LocalNotificationPresenter.ensureChannelCreated(context)
        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val percent = (progress * 100).toInt().coerceIn(0, 100)
        val builder = NotificationCompat.Builder(context, LocalNotificationPresenter.UPDATE_CHANNEL_ID)
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle("Downloading update…")
            .setContentText("Console ${manifest.versionName} ($percent%)")
            .setProgress(100, percent, progress <= 0f)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        runCatching { nm.notify(UPDATE_NOTIFICATION_ID, builder.build()) }
    }

    private fun showDownloadCompleteNotification(context: Context, manifest: UpdateManifest) {
        LocalNotificationPresenter.ensureChannelCreated(context)
        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        val intent = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            putExtra(EXTRA_INSTALL_UPDATE, true)
        }
        val pendingIntent = PendingIntent.getActivity(
            context,
            UPDATE_NOTIFICATION_ID,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )

        val builder = NotificationCompat.Builder(context, LocalNotificationPresenter.UPDATE_CHANNEL_ID)
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle("Update downloaded")
            .setContentText("Console ${manifest.versionName} is ready to install. Tap to proceed.")
            .setStyle(
                NotificationCompat.BigTextStyle()
                    .bigText("Console ${manifest.versionName} is ready. Tap to install the update."),
            )
            .setAutoCancel(true)
            .setOngoing(false)
            .setContentIntent(pendingIntent)
            .setPriority(NotificationCompat.PRIORITY_HIGH)

        runCatching { nm.notify(UPDATE_NOTIFICATION_ID, builder.build()) }
    }

    private fun cancelUpdateNotification(context: Context) {
        val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        runCatching { nm.cancel(UPDATE_NOTIFICATION_ID) }
    }
}
