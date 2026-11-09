package com.console.mobile.feature.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.BuildConfig
import com.console.mobile.core.update.AppUpdateManager
import com.console.mobile.core.update.UpdateState
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.theme.NewTheme

/** Check for / download / install a newer release APK. Release builds only: a debug build is a different app id. */
@Composable
fun AppUpdateSection() {
    if (BuildConfig.DEBUG) return
    val current by AppUpdateManager.state.collectAsStateWithLifecycle()

    val summary = when (current) {
        UpdateState.Idle -> "Version ${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})"
        UpdateState.Checking -> "Checking for updates…"
        UpdateState.UpToDate -> "You're on the latest version (${BuildConfig.VERSION_NAME}, build ${BuildConfig.VERSION_CODE})."
        is UpdateState.Available -> "Update available: ${(current as UpdateState.Available).manifest.versionName} (build ${(current as UpdateState.Available).manifest.versionCode})."
        is UpdateState.Downloading -> "Downloading update… ${((current as UpdateState.Downloading).progress * 100).toInt()}%"
        is UpdateState.ReadyToInstall -> "Update downloaded: ${(current as UpdateState.ReadyToInstall).manifest.versionName}. Ready to install."
        UpdateState.AllowInstalls -> "Allow installs from Console in the settings page that opened, then tap Install again."
        is UpdateState.Failed -> (current as UpdateState.Failed).message
    }
    val busy = current is UpdateState.Checking || current is UpdateState.Downloading
    val readyToInstall = current is UpdateState.Available || current is UpdateState.AllowInstalls || current is UpdateState.ReadyToInstall

    Section("App") {
        Column(Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp)) {
            Text(
                summary,
                color = if (current is UpdateState.Failed) NewTheme.Danger else NewTheme.TextSecondary,
                fontSize = 14.sp,
                modifier = Modifier.padding(bottom = 12.dp),
            )
            ActionButton(
                text = if (readyToInstall) "Install update" else "Check for updates",
                onClick = {
                    when (val state = current) {
                        is UpdateState.Available -> AppUpdateManager.downloadAndInstall(state.manifest)
                        is UpdateState.ReadyToInstall, UpdateState.AllowInstalls -> AppUpdateManager.installPendingUpdate()
                        else -> AppUpdateManager.checkForUpdate()
                    }
                },
                kind = if (readyToInstall) ActionButtonKind.Primary else ActionButtonKind.Secondary,
                enabled = !busy,
                loading = busy,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

