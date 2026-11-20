package com.console.mobile.core.update

import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.common.new.ConfirmPrompt

@Composable
fun UpdatePromptHost() {
    val prompt by AppUpdateManager.prompt.collectAsStateWithLifecycle()
    val item = prompt ?: return

    when (item) {
        is UpdatePrompt.Available -> {
            ConfirmPrompt(
                title = "Update Available",
                message = "Console ${item.manifest.versionName} is available. Would you like to download it now?",
                buttons = listOf(
                    ConfirmButton(text = "Later", cancel = true, onPress = { AppUpdateManager.dismissPrompt() }),
                    ConfirmButton(text = "Download", onPress = {
                        AppUpdateManager.dismissPrompt()
                        AppUpdateManager.downloadAndInstall(item.manifest)
                    }),
                ),
                onDismiss = { AppUpdateManager.dismissPrompt() },
            )
        }

    }
}
