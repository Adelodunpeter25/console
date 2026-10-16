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
import io.github.lyxnx.compose.ui.tablericons.outline.Key
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.ProviderIcon
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.launch

/**
 * A provider whose credential is a key the user pastes once, rather than an
 * OAuth session the server renews. Rendered inside the providers list, where
 * the catalog reports `auth_method == "api-key"`.
 *
 * Three states, because the third one matters:
 *  - no key: the provider still works on its free tier, so the row says so
 *    rather than implying something is broken;
 *  - key stored: "Using your API key";
 *  - key supplied by an environment variable: removing it here cannot take
 *    effect, so the row says where the key comes from and offers no remove
 *    button that would silently do nothing.
 *
 * The key is typed once, sent to the server, and dropped. Nothing on the device
 * needs to read it back, so it is never kept in app state — and no endpoint
 * would return it if something tried.
 */
@Composable
fun ApiKeyProviderRow(
    providerId: String,
    displayName: String,
    description: String,
    modifier: Modifier = Modifier,
) {
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val status = authState.providerCredentials[providerId]
    val configured = status?.configured == true
    val fromEnv = status?.source == "env"
    val busy = providerId in authState.credentialsBusy
    var sheetOpen by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()

    Column(modifier = modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            ProviderIcon(provider = providerId, sizeDp = 24)
            Column(modifier = Modifier.weight(1f).padding(start = 16.dp, end = 12.dp)) {
                Text(
                    displayName.ifBlank { providerId },
                    color = NewTheme.TextPrimary, fontSize = 17.sp,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
                Text(
                    when {
                        fromEnv -> "Using API key from server environment"
                        configured -> "Using your API key"
                        else -> "Using the free tier"
                    },
                    color = if (configured) NewTheme.Success else NewTheme.TextMuted,
                    fontSize = 13.sp,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 1.dp),
                )
            }
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                // An env-supplied key is not ours to remove: a disconnect button
                // here would appear to work and change nothing.
                if (configured && !fromEnv) {
                    ActionButton(
                        text = "Remove",
                        onClick = {
                            confirmAlert(
                                "Remove API key",
                                "$displayName will go back to its free tier. Runs already in flight are unaffected.",
                                listOf(
                                    ConfirmButton("Cancel", cancel = true),
                                    ConfirmButton("Remove", destructive = true, onPress = {
                                        scope.launch {
                                            try {
                                                AppContainer.authRepository.clearProviderCredential(providerId)
                                            } catch (e: Exception) {
                                                confirmAlert("Remove Failed", e.message ?: "Couldn't remove the key.")
                                            }
                                        }
                                    }),
                                ),
                            )
                        },
                        enabled = !busy,
                        loading = busy,
                        kind = ActionButtonKind.Danger,
                        surface = NewTheme.Raised,
                        compact = true,
                    )
                }
                ActionButton(
                    text = if (configured) "Replace" else "Add key",
                    onClick = { sheetOpen = true },
                    enabled = !busy,
                    icon = if (configured) TablerIcons.Outline.Refresh else TablerIcons.Outline.Key,
                    kind = if (configured) ActionButtonKind.Secondary else ActionButtonKind.Primary,
                    surface = NewTheme.Raised,
                    compact = true,
                )
            }
        }
        if (description.isNotBlank()) {
            Text(
                description,
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(start = 58.dp, end = 18.dp, bottom = 12.dp),
            )
        }
    }

    if (sheetOpen) {
        ApiKeySheet(providerId, displayName, onDismiss = { sheetOpen = false })
    }
}

/**
 * Paste sheet. Closes on success and clears the key from memory first, the same
 * way the GitHub token sheet does.
 */
@Composable
private fun ApiKeySheet(providerId: String, displayName: String, onDismiss: () -> Unit) {
    var key by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    BaseSheet(onDismiss = { if (!busy) onDismiss() }, title = "$displayName API key") {
        Note("Optional. Without a key this provider uses its free tier. A key raises limits and can unlock paid models.")
        TextInput(
            label = "API key",
            value = key,
            onValueChange = { key = it; error = null },
            placeholder = "sk-…",
            monospace = true,
            secret = true,
        )
        error?.let { Banner(it, modifier = Modifier.padding(top = 12.dp)) }
        ActionButton(
            text = "Save",
            onClick = {
                busy = true
                error = null
                scope.launch {
                    try {
                        AppContainer.authRepository.setProviderCredential(providerId, key.trim())
                        // Done with the key: drop it before the sheet closes.
                        key = ""
                        onDismiss()
                    } catch (e: Exception) {
                        error = e.message ?: "Couldn't save the key."
                    } finally {
                        busy = false
                    }
                }
            },
            enabled = key.isNotBlank() && !busy,
            loading = busy,
            kind = ActionButtonKind.Primary,
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp),
        )
    }
}