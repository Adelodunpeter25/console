package com.console.mobile.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Login
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.launch

/**
 * Speech-to-text voice dictation, as a row under the AI providers. It is not
 * a chat provider: the pair (Cloudflare account ID + API token) lets the
 * server transcribe recordings with a Whisper model. So it has its own
 * status, not the provider catalog's, and its own sheet instead of an OAuth
 * browser trip.
 *
 * The pair is typed once, sent to the server, and dropped — what comes back
 * is only whether one is configured and where it came from.
 */
@Composable
fun SttAccountSection() {
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val stt = authState.sttStatus
    val configured = stt?.configured == true
    val fromEnv = stt?.source == "env"
    var sheetOpen by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    Section("Speech to Text") {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
                Text("Speech to Text", color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1)
                Text(
                    when {
                        fromEnv -> "Connected — managed by the server"
                        configured -> "Connected"
                        else -> "Not connected"
                    },
                    color = if (configured) NewTheme.Success else NewTheme.TextMuted,
                    fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 1.dp),
                )
                if (configured) {
                    Text(
                        "Transcribes recordings with ${stt.model.ifBlank { "Whisper" }}.",
                        color = NewTheme.TextMuted,
                        fontSize = 12.sp,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                // An environment-supplied pair is not ours to remove: a
                // disconnect button would appear to work and change nothing.
                if (configured && !fromEnv) {
                    ActionButton(
                        text = "Disconnect",
                        onClick = {
                            confirmAlert(
                                "Disconnect Speech to Text",
                                "Dictation will stop working until you connect again.",
                                listOf(
                                    ConfirmButton("Cancel", cancel = true),
                                    ConfirmButton("Disconnect", destructive = true, onPress = {
                                        scope.launch {
                                            try {
                                                AppContainer.authRepository.clearSttConfig()
                                            } catch (e: Exception) {
                                                confirmAlert("Disconnect Failed", e.message ?: "Couldn't disconnect.")
                                            }
                                        }
                                    }),
                                ),
                            )
                        },
                        enabled = !authState.sttBusy,
                        loading = authState.sttBusy,
                        kind = ActionButtonKind.Danger,
                        surface = NewTheme.Raised,
                        compact = true,
                    )
                }
                ActionButton(
                    text = if (configured) "Replace" else "Connect",
                    onClick = { sheetOpen = true },
                    enabled = !authState.sttBusy && !fromEnv,
                    icon = if (configured) TablerIcons.Outline.Refresh else TablerIcons.Outline.Login,
                    kind = if (configured) ActionButtonKind.Secondary else ActionButtonKind.Primary,
                    surface = NewTheme.Raised,
                    compact = true,
                )
            }
        }
    }
    Note("Paste a Cloudflare account ID and API token. Recordings are transcribed on the server with a Whisper model — nothing is stored from the audio itself.")

    if (sheetOpen) SttConfigSheet(onDismiss = { sheetOpen = false })
}

/**
 * Connect sheet. Both halves are required every time: the token is never
 * returned, so a partial update cannot preserve it. Clears both fields on
 * submit, the same way the GitHub and API-key sheets do.
 */
@Composable
private fun SttConfigSheet(onDismiss: () -> Unit) {
    var accountId by remember { mutableStateOf("") }
    var apiToken by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    BaseSheet(onDismiss = { if (!busy) onDismiss() }, title = "Connect Speech to Text") {
        Note("Stored on this server only. The token is never shown again — to replace it, disconnect and connect with the new pair.")
        TextInput(
            label = "Cloudflare account ID",
            value = accountId,
            onValueChange = { accountId = it; error = null },
            placeholder = "e.g. e32ab932dec1b35e0df9c0ec56ab6c8f",
            monospace = true,
        )
        TextInput(
            label = "API token",
            value = apiToken,
            onValueChange = { apiToken = it; error = null },
            placeholder = "Workers AI API token",
            monospace = true,
            secret = true,
        )
        error?.let { Banner(it, modifier = Modifier.padding(top = 12.dp)) }
        ActionButton(
            text = "Connect",
            onClick = {
                busy = true
                error = null
                scope.launch {
                    try {
                        AppContainer.authRepository.setSttConfig(accountId.trim(), apiToken.trim())
                        // Done with the pair: drop both before the sheet closes.
                        accountId = ""
                        apiToken = ""
                        onDismiss()
                    } catch (e: Exception) {
                        error = e.message ?: "Couldn't save the credentials."
                    } finally {
                        busy = false
                    }
                }
            },
            enabled = accountId.isNotBlank() && apiToken.isNotBlank() && !busy,
            loading = busy,
            kind = ActionButtonKind.Primary,
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
        )
    }
}
