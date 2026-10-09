package com.console.mobile.feature.settings

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
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
 * The GitHub git credential, as a row under the AI providers. It is not a chat
 * provider: a personal access token lets the server's `git` (agent runs and
 * terminals) reach private repositories. So it has its own status, not the
 * provider catalog's, and its own paste sheet instead of an OAuth browser trip.
 *
 * The token is typed once, sent to the server, and dropped — only the
 * username comes back.
 */
@Composable
fun GitHubAccountSection() {
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val github = authState.github
    val connected = github?.connected == true
    var sheetOpen by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    Section("Git") {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
                Text("GitHub", color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1)
                Text(
                    if (connected) github?.username?.takeIf { it.isNotBlank() }?.let { "@$it" } ?: "Connected" else "Not connected",
                    color = if (connected) NewTheme.Success else NewTheme.TextMuted,
                    fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 1.dp),
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                if (connected) {
                    ActionButton(
                        text = "Disconnect",
                        onClick = {
                            confirmAlert(
                                "Disconnect GitHub",
                                "Git on the server will stop working with private repositories until you connect again.",
                                listOf(
                                    ConfirmButton("Cancel", cancel = true),
                                    ConfirmButton("Disconnect", destructive = true, onPress = {
                                        scope.launch {
                                            try {
                                                AppContainer.authRepository.disconnectGitHub()
                                            } catch (e: Exception) {
                                                confirmAlert("Disconnect Failed", e.message ?: "Couldn't disconnect GitHub.")
                                            }
                                        }
                                    }),
                                ),
                            )
                        },
                        enabled = !authState.githubBusy,
                        loading = authState.githubBusy,
                        kind = ActionButtonKind.Danger,
                        surface = NewTheme.Raised,
                        compact = true,
                    )
                }
                ActionButton(
                    text = if (connected) "Re-login" else "Connect",
                    onClick = { sheetOpen = true },
                    enabled = !authState.githubBusy,
                    icon = if (connected) TablerIcons.Outline.Refresh else TablerIcons.Outline.Login,
                    kind = if (connected) ActionButtonKind.Secondary else ActionButtonKind.Primary,
                    surface = NewTheme.Raised,
                    compact = true,
                )
            }
        }
    }
    Note("A personal access token lets git on the server clone and push private repositories, from the agent and the terminal.")

    if (sheetOpen) GitHubTokenSheet(onDismiss = { sheetOpen = false })
}

/** Paste sheet: validates the token on the server and closes on success. */
@Composable
private fun GitHubTokenSheet(onDismiss: () -> Unit) {
    var token by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    BaseSheet(onDismiss = { if (!busy) onDismiss() }, title = "Connect GitHub") {
        Note("Paste a personal access token. A fine-grained token with access to your repositories works, as does a classic token with the repo scope.")
        TextInput(
            label = "Token",
            value = token,
            onValueChange = { token = it; error = null },
            placeholder = "github_pat_… or ghp_…",
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
                        AppContainer.authRepository.connectGitHub(token)
                        // Done with the token: drop it before the sheet closes.
                        token = ""
                        onDismiss()
                    } catch (e: Exception) {
                        error = e.message ?: "Couldn't connect GitHub."
                    } finally {
                        busy = false
                    }
                }
            },
            enabled = token.isNotBlank() && !busy,
            loading = busy,
            kind = ActionButtonKind.Primary,
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
        )
    }
}
