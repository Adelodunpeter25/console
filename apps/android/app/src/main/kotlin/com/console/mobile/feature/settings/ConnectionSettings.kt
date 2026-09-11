package com.console.mobile.feature.settings

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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.LinkOff
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
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
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHost
import com.console.mobile.data.store.Environment
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit

/**
 * Port of screens/settings/environments-settings.tsx + components/environments/environment-editor.tsx.
 * List → inline add/edit form (same screen, like Expo). Probe dots via GET /api/projects.
 */
@Composable
fun ConnectionSettings(onBack: () -> Unit) {
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<String?>(null) } // null=list, "__create__"=create, else env id
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

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
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
                    TextButton(onClick = { editing = "__create__" }) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Filled.Add, contentDescription = null, tint = ConsoleColors.TextPrimary, modifier = Modifier.size(14.dp))
                            Text("Add", color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 4.dp))
                        }
                    }
                },
            )
            Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
                val cardShape = RoundedCornerShape(16.dp)
                Column(modifier = Modifier.fillMaxWidth().clip(cardShape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, cardShape).padding(16.dp)) {
                    if (envState.environments.isEmpty()) {
                        Text("No environments yet. Add a backend URL to get started.", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 12.dp))
                    } else {
                        envState.environments.forEach { env ->
                            val probe = probes[env.id]
                            val isActive = env.id == envState.activeId
                            Row(
                                modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).clickable { editing = env.id }.padding(horizontal = 12.dp, vertical = 10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(when (probe) { null -> Color(0xFF52525B); true -> Color(0xFF34D399); else -> Color(0xFFF87171) }))
                                Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Text(env.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                                        if (isActive) {
                                            Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(999.dp)).background(Color(0xFF34D399).copy(alpha = 0.15f)).padding(horizontal = 8.dp, vertical = 2.dp)) {
                                                Text("Active", color = Color(0xFF34D399), fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
                                            }
                                        }
                                    }
                                    Text(urlHost(env.url), color = ConsoleColors.TextSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                                }
                                Icon(Icons.Filled.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(18.dp))
                            }
                        }
                    }
                }
                if (envState.activeId != null) {
                    TextButton(
                        onClick = {
                            confirmAlert("Disconnect Backend", "Are you sure you want to disconnect? This removes all environments and connection data, like a clean install.", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Disconnect", destructive = true, onPress = {
                                scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.deactivate() } }
                            })))
                        },
                        modifier = Modifier.fillMaxWidth().padding(top = 16.dp).clip(RoundedCornerShape(16.dp)).background(ConsoleColors.Destructive.copy(alpha = 0.05f)).border(1.dp, ConsoleColors.Destructive.copy(alpha = 0.3f), RoundedCornerShape(16.dp)).padding(vertical = 12.dp),
                    ) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Filled.LinkOff, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(16.dp))
                            Text("Disconnect backend", color = ConsoleColors.Destructive, fontSize = 14.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp))
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

    val fieldColors = OutlinedTextFieldDefaults.colors(
        focusedContainerColor = ConsoleColors.CardAlt,
        unfocusedContainerColor = ConsoleColors.CardAlt,
        focusedBorderColor = ConsoleColors.Border,
        unfocusedBorderColor = ConsoleColors.BorderSubtle,
        focusedTextColor = ConsoleColors.TextPrimary,
        unfocusedTextColor = ConsoleColors.TextPrimary,
        cursorColor = ConsoleColors.TextPrimary,
        focusedPlaceholderColor = ConsoleColors.TextMuted,
        unfocusedPlaceholderColor = ConsoleColors.TextMuted,
        focusedLabelColor = ConsoleColors.TextSecondary,
        unfocusedLabelColor = ConsoleColors.TextSecondary,
    )

    Column(modifier = modifier) {
        Text("Name", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(bottom = 6.dp))
        OutlinedTextField(value = name, onValueChange = { name = it; if (status != "testing" && status != "saving") status = "idle" }, placeholder = { Text("My server") }, singleLine = true, colors = fieldColors, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 16.dp))
        Text("Backend URL", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(bottom = 6.dp))
        OutlinedTextField(value = url, onValueChange = { url = it; if (status != "testing" && status != "saving") status = "idle" }, placeholder = { Text("http://192.168.1.X:3000") }, singleLine = true, colors = fieldColors, shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth())
        TextButton(
            onClick = {
                val normalized = normalizeBackendUrl(url) ?: return@TextButton
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
            modifier = Modifier.fillMaxWidth().padding(top = 16.dp).clip(RoundedCornerShape(12.dp)).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp)).padding(vertical = 10.dp),
        ) {
            if (status == "testing") CircularProgressIndicator(color = ConsoleColors.TextSecondary, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
            Text(if (status == "testing") "Testing…" else "Test connection", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium)
        }
        if (status == "test-ok") Text("Connection OK", color = Color(0xFF34D399), fontSize = 12.sp, modifier = Modifier.padding(top = 6.dp))
        else if (status == "test-fail") Text("Could not reach the backend", color = ConsoleColors.Destructive, fontSize = 12.sp, modifier = Modifier.padding(top = 6.dp))
        if (env != null && !isActive) {
            TextButton(
                onClick = {
                    scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.activateEnvironment(env.id) } }
                    onDone()
                },
                modifier = Modifier.fillMaxWidth().padding(top = 16.dp).clip(RoundedCornerShape(12.dp)).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp)).padding(vertical = 10.dp),
            ) { Text("Set as active", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium) }
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 20.dp)) {
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
            TextButton(
                onClick = doSave,
                enabled = url.isNotBlank() && status != "saving",
                modifier = Modifier.weight(1f).clip(RoundedCornerShape(12.dp)).background(if (url.isNotBlank()) Color.White else Color.White.copy(alpha = 0.4f)).padding(vertical = 12.dp),
            ) {
                if (status == "saving") CircularProgressIndicator(color = Color.Black, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                else Text(if (env != null) "Save changes" else "Save", color = Color.Black, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
            }
            if (env != null) {
                TextButton(
                    onClick = {
                        confirmAlert("Delete environment", "Remove \"${env.name}\" from your environments?", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Delete", destructive = true, onPress = {
                            scope.launch { withContext(Dispatchers.IO) { AppContainer.environmentsRepository.removeEnvironment(env.id) } }
                            onDone()
                        })))
                    },
                    enabled = canDelete,
                    modifier = Modifier.weight(1f).padding(start = 12.dp).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Destructive.copy(alpha = 0.05f)).border(1.dp, ConsoleColors.Destructive.copy(alpha = 0.3f), RoundedCornerShape(12.dp)).padding(vertical = 12.dp),
                ) { Text("Delete", color = ConsoleColors.Destructive, fontSize = 14.sp, fontWeight = FontWeight.Medium) }
            }
        }
    }
}
