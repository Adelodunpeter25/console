package com.console.mobile.feature.home

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowNarrowLeft
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Server
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHostPort
import com.console.mobile.data.model.deviceName
import com.console.mobile.data.model.splitHostAndCode
import com.console.mobile.data.store.Environment
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextFieldRow
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.SectionCard

/**
 * Port of components/environments/environment-switcher.tsx.
 * Server-icon trigger → bottom sheet with env list + probe dots, inline add form.
 * Probes are local UI state (GET {url}/api/projects, 6s) — mirrors probeEnvironment.
 *
 * The add form asks for a host and a pairing code, the same two things the
 * server prints: a code means the server wants this phone to pair, and the
 * phone exchanges it for its own credential before it starts talking to
 * the server.
 */
@Composable
fun EnvironmentSwitcher(modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var sheetOpen by remember { mutableStateOf(false) }
    var creating by remember { mutableStateOf(false) }
    var probes by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }
    var newHost by remember { mutableStateOf("") }
    var newCode by remember { mutableStateOf("") }
    var adding by remember { mutableStateOf(false) }
    var failure by remember { mutableStateOf<String?>(null) }

    fun probeAll(envs: List<Environment>) {
        scope.launch {
            val results = withContext(Dispatchers.IO) {
                envs.associate { env ->
                    // The probe carries that server's own credential: a
                    // paired server answers 401 without it, and the dot
                    // would show every paired server as unreachable.
                    env.id to AppContainer.authRepository.probeServer(env.url)
                }
            }
            probes = results
        }
    }

    HeaderIconButton(
        icon = TablerIcons.Outline.Server,
        contentDescription = "Switch environment",
        onClick = {
            creating = false
            sheetOpen = true
            probeAll(envState.environments)
        },
        modifier = modifier,
    )

    if (sheetOpen) {
        BaseSheet(onDismiss = { sheetOpen = false }, title = if (creating) "Add environment" else "Environments") {
            if (creating) {
                Column(modifier = Modifier.fillMaxWidth()) {
                    ActionRow(icon = TablerIcons.Outline.ArrowNarrowLeft, title = "Back to environments") { creating = false }
                    SectionCard {
                        TextFieldRow(
                            "Host",
                            newHost,
                            { text ->
                                // A pasted pairing URL carries the code in
                                // its fragment, so the field splits it into
                                // the two.
                                val (address, pasted) = splitHostAndCode(text)
                                newHost = address
                                if (pasted.isNotBlank()) newCode = pasted
                                failure = null
                            },
                            "192.168.1.100:8080",
                            monospace = true,
                        )
                        SectionDivider(startInset = 18.dp)
                        TextFieldRow(
                            "Pairing code",
                            newCode,
                            { newCode = it; failure = null },
                            "abc-123-xyz",
                            monospace = true,
                        )
                    }
                    failure?.let { message ->
                        Text(message, color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
                    }
                    ActionButton(
                        text = if (adding) "Adding…" else "Add environment",
                        onClick = {
                            if (adding) return@ActionButton
                            val normalized = normalizeBackendUrl(newHost) ?: run {
                                failure = "That host cannot be used."
                                return@ActionButton
                            }
                            adding = true
                            failure = null
                            scope.launch {
                                try {
                                    // A code means the server wants this phone
                                    // to pair: exchange it for a credential
                                    // before the app starts talking to the
                                    // new address.
                                    var serverName = ""
                                    if (newCode.isNotBlank()) {
                                        val paired = AppContainer.authRepository.pairWithCode(normalized, newCode, deviceName())
                                        serverName = paired.server?.name?.takeIf { it.isNotBlank() }.orEmpty()
                                    }
                                    val label = serverName.ifBlank { urlHostPort(normalized) }
                                    withContext(Dispatchers.IO) {
                                        AppContainer.environmentsRepository.activateUrl(label, normalized)
                                    }
                                    newHost = ""
                                    newCode = ""
                                    creating = false
                                    sheetOpen = false
                                } catch (e: Exception) {
                                    failure = e.message ?: "Could not add the server."
                                } finally {
                                    adding = false
                                }
                            }
                        },
                        kind = ActionButtonKind.Primary,
                        enabled = !adding && newHost.isNotBlank(),
                        loading = adding,
                        modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
                    )
                }
            } else {
                Column(modifier = Modifier.fillMaxWidth()) {
                    SectionCard {
                        envState.environments.forEachIndexed { index, env ->
                            val isActive = env.id == envState.activeId
                            val probe = probes[env.id]
                            Row(
                                modifier = Modifier.fillMaxWidth()
                                    .clickable {
                                        if (env.id != envState.activeId) {
                                            scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.activateEnvironment(env.id) } }
                                        }
                                        sheetOpen = false
                                    }
                                    .padding(horizontal = 18.dp, vertical = 16.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                // Reachability: grey until probed, then green/red.
                                Box(modifier = Modifier.size(10.dp).clip(CircleShape).background(when (probe) { null -> NewTheme.TextGhost; true -> NewTheme.Success; false -> NewTheme.Danger }))
                                Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
                                    Text(env.name, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    Text(urlHostPort(env.url), color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
                                }
                                if (isActive) Icon(TablerIcons.Outline.Check, contentDescription = "Active", tint = NewTheme.Success)
                            }
                            if (index < envState.environments.lastIndex) SectionDivider(startInset = 44.dp)
                        }
                        if (envState.environments.isNotEmpty()) SectionDivider()
                        ActionRow(icon = TablerIcons.Outline.Plus, title = "Add environment") { creating = true }
                    }
                }
            }
        }
    }
}
