package com.console.mobile.feature.settings

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.BuildConfig
import com.console.mobile.core.update.AppUpdater
import com.console.mobile.core.update.UpdateManifest
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private sealed interface UpdateState {
    data object Idle : UpdateState
    data object Checking : UpdateState
    data object UpToDate : UpdateState
    data class Available(val manifest: UpdateManifest) : UpdateState
    data class Downloading(val progress: Float) : UpdateState
    data object AllowInstalls : UpdateState
    data class Failed(val message: String) : UpdateState
}

/** Check for / download / install a newer release APK. Release builds only: a debug build is a different app id. */
@Composable
fun AppUpdateSection() {
    if (BuildConfig.DEBUG) return
    var state by remember { mutableStateOf<UpdateState>(UpdateState.Idle) }
    val scope = rememberCoroutineScope()

    fun check() {
        state = UpdateState.Checking
        scope.launch {
            state = try {
                val manifest = withContext(Dispatchers.IO) { AppUpdater.checkForUpdate() }
                if (manifest == null) UpdateState.UpToDate else UpdateState.Available(manifest)
            } catch (e: Exception) {
                UpdateState.Failed(e.message ?: "Unable to check for updates.")
            }
        }
    }

    fun install() {
        state = UpdateState.Downloading(0f)
        scope.launch {
            state = try {
                val apk = withContext(Dispatchers.IO) {
                    AppUpdater.download { state = UpdateState.Downloading(it) }
                }
                if (AppUpdater.install(apk)) UpdateState.Idle else UpdateState.AllowInstalls
            } catch (e: Exception) {
                UpdateState.Failed(e.message ?: "Unable to download the update.")
            }
        }
    }

    val current = state
    val summary = when (current) {
        UpdateState.Idle -> "Version ${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})"
        UpdateState.Checking -> "Checking for updates…"
        UpdateState.UpToDate -> "You're on the latest version (${BuildConfig.VERSION_NAME}, build ${BuildConfig.VERSION_CODE})."
        is UpdateState.Available -> "Update available: ${current.manifest.versionName} (build ${current.manifest.versionCode})."
        is UpdateState.Downloading -> "Downloading update… ${(current.progress * 100).toInt()}%"
        UpdateState.AllowInstalls -> "Allow installs from Console in the settings page that opened, then tap Install again."
        is UpdateState.Failed -> current.message
    }
    val busy = current is UpdateState.Checking || current is UpdateState.Downloading

    Section("App") {
        Column(Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp)) {
            Text(
                summary,
                color = if (current is UpdateState.Failed) NewTheme.Danger else NewTheme.TextSecondary,
                fontSize = 14.sp,
                modifier = Modifier.padding(bottom = 12.dp),
            )
            val readyToInstall = current is UpdateState.Available || current is UpdateState.AllowInstalls
            ActionButton(
                text = if (readyToInstall) "Install update" else "Check for updates",
                onClick = { if (readyToInstall) install() else check() },
                kind = if (readyToInstall) ActionButtonKind.Primary else ActionButtonKind.Secondary,
                enabled = !busy,
                loading = busy,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
