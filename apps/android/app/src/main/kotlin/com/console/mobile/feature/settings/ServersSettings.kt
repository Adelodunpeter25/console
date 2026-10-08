package com.console.mobile.feature.settings

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.LinkOff
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHostPort
import com.console.mobile.data.store.Environment
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.PillButtonVariant
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextInput

/**
 * Port of screens/settings/environments-settings.tsx + components/environments/environment-editor.tsx.
 * List → inline add/edit form (same screen, like Expo). Probe dots via GET /api/projects.
 */
@Composable
fun ServersSettings(onBack: () -> Unit) {
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<String?>(null) } // null=list, "__create__"=create, else env id
    BackHandler(enabled = editing != null) { editing = null }
    var probes by remember { mutableStateOf<Map<String, Boolean>>(emptyMap()) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(envState.environments) {
        val envs = envState.environments
        if (envs.isEmpty()) return@LaunchedEffect
        val results = withContext(Dispatchers.IO) {
            envs.associate { env ->
                env.id to try {
                    val client = OkHttpClient.Builder().connectTimeout(6, TimeUnit.SECONDS).readTimeout(6, TimeUnit.SECONDS).callTimeout(6, TimeUnit.SECONDS).build()
                    val req = Request.Builder().url("${env.url.trimEnd('/')}/api/projects").get().build()
                    client.newCall(req).execute().use { it.isSuccessful }
                } catch (_: Exception) { false }
            }
        }
        probes = results
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        if (editing != null) {
            val envId = editing!!.takeIf { it != "__create__" }
            val editingEnv = envState.environments.firstOrNull { it.id == envId }
            ScreenHeader(title = if (envId != null) "Edit environment" else "Add environment", onBack = { editing = null })
            EnvironmentEditorForm(env = editingEnv, onDone = { editing = null }, modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp))
        } else {
            ScreenHeader(
                title = "Connection",
                onBack = onBack,
                actions = {
                    IconButton(onClick = { editing = "__create__" }, modifier = Modifier.size(40.dp)) {
                        Box(
                            modifier = Modifier.size(40.dp).clip(CircleShape).background(NewTheme.Card),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(TablerIcons.Outline.Plus, contentDescription = "Add environment", tint = NewTheme.TextPrimary)
                        }
                    }
                },
            )
            Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
                Section("Environments") {
                    if (envState.environments.isEmpty()) {
                        Note("No environments yet. Add a backend URL to get started.")
                    } else {
                        envState.environments.forEachIndexed { index, env ->
                            val probe = probes[env.id]
                            val isActive = env.id == envState.activeId
                            Row(
                                modifier = Modifier.fillMaxWidth().clickable { editing = env.id }.padding(horizontal = 18.dp, vertical = 16.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                // Reachability: grey until probed, then green/red.
                                Box(modifier = Modifier.size(10.dp).clip(CircleShape).background(when (probe) { null -> NewTheme.TextGhost; true -> NewTheme.Success; else -> NewTheme.Danger }))
                                Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Text(env.name, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                                        if (isActive) {
                                            Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(999.dp)).background(NewTheme.Success.copy(alpha = 0.15f)).padding(horizontal = 8.dp, vertical = 2.dp)) {
                                                Text("Active", color = NewTheme.Success, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
                                            }
                                        }
                                    }
                                    Text(urlHostPort(env.url), color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
                                }
                                Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(20.dp))
                            }
                            if (index < envState.environments.lastIndex) SectionDivider(startInset = 44.dp)
                        }
                    }
                }
                if (envState.activeId != null) {
                    Section("Connection") {
                        ActionRow(icon = TablerIcons.Outline.LinkOff, title = "Disconnect backend", tint = NewTheme.Danger) {
                            confirmAlert("Disconnect Backend", "Are you sure you want to disconnect? This removes all environments and connection data, like a clean install.", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Disconnect", destructive = true, onPress = {
                                scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.deactivate() } }
                            })))
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun EnvironmentEditorForm(env: Environment?, onDone: () -> Unit, modifier: Modifier = Modifier) {
    val scope = rememberCoroutineScope()
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var name by remember(env?.id) { mutableStateOf(env?.name ?: "") }
    var url by remember(env?.id) { mutableStateOf(env?.url ?: "") }
    var status by remember(env?.id) { mutableStateOf("idle") } // idle|testing|test-ok|test-fail|saving
    val isActive = env != null && envState.activeId == env.id
    val canDelete = env != null && !(isActive && envState.environments.size == 1)

    Column(modifier = modifier) {
        TextInput("Name", name, { name = it; if (status != "testing" && status != "saving") status = "idle" }, "My server")
        TextInput("Backend URL", url, { url = it; if (status != "testing" && status != "saving") status = "idle" }, "http://192.168.1.X:3000")
        ActionButton(
            text = if (status == "testing") "Testing…" else "Test connection",
            onClick = {
                val normalized = normalizeBackendUrl(url) ?: return@ActionButton
                status = "testing"
                scope.launch {
                    val ok = withContext(Dispatchers.IO) {
                        try {
                            val client = OkHttpClient.Builder().connectTimeout(6, TimeUnit.SECONDS).readTimeout(6, TimeUnit.SECONDS).callTimeout(6, TimeUnit.SECONDS).build()
                            val req = Request.Builder().url("${normalized.trimEnd('/')}/api/projects").get().build()
                            client.newCall(req).execute().use { it.isSuccessful }
                        } catch (_: Exception) { false }
                    }
                    status = if (ok) "test-ok" else "test-fail"
                }
            },
            enabled = url.isNotBlank() && status != "testing",
            loading = status == "testing",
            modifier = Modifier.fillMaxWidth().padding(top = 20.dp),
        )
        if (status == "test-ok") Text("Connection OK", color = NewTheme.Success, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        else if (status == "test-fail") Text("Could not reach the backend", color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        if (env != null && !isActive) {
            ActionButton(
                text = "Set as active",
                onClick = {
                    scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.activateEnvironment(env.id) } }
                    onDone()
                },
                modifier = Modifier.fillMaxWidth().padding(top = 12.dp),
            )
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 24.dp)) {
            val doSave: () -> Unit = {
                val normalized = normalizeBackendUrl(url)
                if (normalized == null) {
                    confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.")
                } else if (status != "test-ok") {
                    confirmAlert("Untested environment", "Save without a successful connection test?", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Save", onPress = {
                        scope.launch {
                            status = "saving"
                            try {
                                withContext(Dispatchers.IO) {
                                    if (env != null) AppContainer.environmentsRepository.updateEnvironment(env.id, name.ifBlank { env.name }, normalized)
                                    else AppContainer.environmentsRepository.addEnvironment(name.ifBlank { "Default" }, normalized)
                                }
                                onDone()
                            } catch (e: Exception) {
                                confirmAlert("Error", e.message ?: "Failed to save.")
                            } finally { status = "idle" }
                        }
                    })))
                } else {
                    scope.launch {
                        status = "saving"
                        try {
                            withContext(Dispatchers.IO) {
                                if (env != null) AppContainer.environmentsRepository.updateEnvironment(env.id, name.ifBlank { env.name }, normalized)
                                else AppContainer.environmentsRepository.addEnvironment(name.ifBlank { "Default" }, normalized)
                            }
                            onDone()
                        } catch (e: Exception) {
                            confirmAlert("Error", e.message ?: "Failed to save.")
                        } finally { status = "idle" }
                    }
                }
            }
            ActionButton(
                text = if (env != null) "Save changes" else "Save",
                onClick = doSave,
                kind = ActionButtonKind.Primary,
                enabled = url.isNotBlank() && status != "saving",
                loading = status == "saving",
                modifier = Modifier.weight(1f),
            )
            if (env != null) {
                ActionButton(
                    text = "Delete",
                    onClick = {
                        confirmAlert("Delete environment", "Remove \"${env.name}\" from your environments?", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Delete", destructive = true, onPress = {
                            scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.removeEnvironment(env.id) } }
                            onDone()
                        })))
                    },
                    kind = ActionButtonKind.Danger,
                    enabled = canDelete,
                    modifier = Modifier.weight(1f).padding(start = 12.dp),
                )
            }
        }
    }
}
