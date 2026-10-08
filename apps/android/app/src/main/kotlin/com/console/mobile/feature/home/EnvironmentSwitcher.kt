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
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
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
import com.console.mobile.data.store.Environment
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.CircleIconButton
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme

/**
 * Port of components/environments/environment-switcher.tsx.
 * Server-icon trigger → bottom sheet with env list + probe dots, inline add form.
 * Probes are local UI state (GET {url}/api/projects, 6s) — mirrors probeEnvironment.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun EnvironmentSwitcher(modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var sheetOpen by remember { mutableStateOf(false) }
    var creating by remember { mutableStateOf(false) }
    var probes by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }

    fun probeAll(envs: List<Environment>) {
        scope.launch {
            val results = withContext(Dispatchers.IO) {
                envs.associate { env ->
                    env.id to try {
                        val client = OkHttpClient.Builder()
                            .connectTimeout(6, TimeUnit.SECONDS)
                            .readTimeout(6, TimeUnit.SECONDS)
                            .callTimeout(6, TimeUnit.SECONDS)
                            .build()
                        val req = Request.Builder().url("${env.url.trimEnd('/')}/api/projects").get().build()
                        client.newCall(req).execute().use { it.isSuccessful }
                    } catch (_: Exception) {
                        false
                    }
                }
            }
            probes = results
        }
    }

    CircleIconButton(
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
        ModalBottomSheet(
            onDismissRequest = { sheetOpen = false },
            sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
            containerColor = NewTheme.Background,
        ) {
            if (creating) {
                var newName by remember { mutableStateOf("") }
                var newUrl by remember { mutableStateOf("") }
                Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(bottom = 4.dp)) {
                        CircleIconButton(TablerIcons.Outline.ArrowNarrowLeft, "Back", { creating = false })
                        Text("Add environment", color = NewTheme.TextPrimary, fontSize = 18.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 12.dp))
                    }
                    TextInput("Name", newName, { newName = it }, "My server")
                    TextInput("Backend URL", newUrl, { newUrl = it }, "http://192.168.1.X:3000")
                    ActionButton(
                        text = "Save",
                        onClick = {
                            val normalized = normalizeBackendUrl(newUrl) ?: return@ActionButton
                            scope.launch {
                                withContext(Dispatchers.IO) {
                                    AppContainer.environmentsRepository.addEnvironment(newName.ifBlank { "Default" }, normalized)
                                }
                                creating = false
                                sheetOpen = false
                            }
                        },
                        kind = ActionButtonKind.Primary,
                        enabled = newUrl.isNotBlank(),
                        modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
                    )
                }
            } else {
                Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
                    Section("Environments") {
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
