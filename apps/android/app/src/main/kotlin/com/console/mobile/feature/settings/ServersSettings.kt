package com.console.mobile.feature.settings

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
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
import io.github.lyxnx.compose.ui.tablericons.outline.Camera
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import com.console.mobile.AppContainer
import com.console.mobile.core.util.normalizeBackendUrl
import com.console.mobile.core.util.urlHostPort
import com.console.mobile.data.store.Environment
import com.console.mobile.data.model.deviceName
import com.console.mobile.data.model.pairingCode
import com.console.mobile.data.model.pairingHost
import com.console.mobile.data.model.splitHostAndCode
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.util.concurrent.TimeUnit
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextFieldRow
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.components.common.new.PageHeader

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

    // The add and edit sheets' server fields. They live here rather than in
    // the forms so the camera button in the header can fill them.
    var pairingHost by remember { mutableStateOf("") }
    var pairingCode by remember { mutableStateOf("") }
    var scanError by remember { mutableStateOf<String?>(null) }

    // Opening a sheet starts from that environment's own details: the
    // address it is at, and no code (a code is used once, and the server
    // hands out a new one whenever it is asked).
    LaunchedEffect(editing) {
        val envId = editing?.takeIf { it != "__create__" }
        val env = envState.environments.firstOrNull { it.id == envId }
        pairingHost = env?.url.orEmpty()
        pairingCode = ""
        scanError = null
    }

    // The QR code the server prints carries the whole pairing URL: the host
    // and the code. Scanning it fills both fields.
    val scannerLauncher = rememberLauncherForActivityResult(ScanContract()) { result ->
        val scanned = result.contents
        if (scanned.isNullOrBlank()) return@rememberLauncherForActivityResult
        val host = pairingHost(scanned)
        val code = pairingCode(scanned)
        if (host == null || code == null) {
            scanError = "That QR code is not a Console pairing code."
        } else {
            pairingHost = host
            pairingCode = code
            scanError = null
        }
    }

    LaunchedEffect(envState.environments) {
        val envs = envState.environments
        if (envs.isEmpty()) return@LaunchedEffect
        val results = withContext(Dispatchers.IO) {
            envs.associate { env ->
                // The probe carries that server's own credential: a paired
                // server answers 401 without it, and the dot would show
                // every paired server as unreachable.
                env.id to AppContainer.authRepository.probeServer(env.url)
            }
        }
        probes = results
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        if (editing != null) {
            val envId = editing!!.takeIf { it != "__create__" }
            val editingEnv = envState.environments.firstOrNull { it.id == envId }
            if (envId == null) {
                // The add sheet: a host and a pairing code, with a camera
                // for reading the QR code the server prints.
                PageHeader(
                    title = "Add environment",
                    onBack = { editing = null },
                    actions = {
                        HeaderIconButton(
                            icon = TablerIcons.Outline.Camera,
                            contentDescription = "Scan pairing QR code",
                            onClick = {
                                scanError = null
                                scannerLauncher.launch(
                                    ScanOptions().apply {
                                        setPrompt("Scan the server's pairing code")
                                        setBeepEnabled(false)
                                        setOrientationLocked(false)
                                    }
                                )
                            },
                        )
                    },
                )
                AddEnvironmentForm(
                    host = pairingHost,
                    code = pairingCode,
                    onHostChange = { pairingHost = it },
                    onCodeChange = { pairingCode = it },
                    error = scanError,
                    onDone = { editing = null },
                    modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp),
                )
            } else {
                PageHeader(
                    title = "Edit environment",
                    onBack = { editing = null },
                    actions = {
                        HeaderIconButton(
                            icon = TablerIcons.Outline.Camera,
                            contentDescription = "Scan pairing QR code",
                            onClick = {
                                scanError = null
                                scannerLauncher.launch(
                                    ScanOptions().apply {
                                        setPrompt("Scan the server's pairing code")
                                        setBeepEnabled(false)
                                        setOrientationLocked(false)
                                    }
                                )
                            },
                        )
                    },
                )
                EnvironmentEditorForm(
                    env = editingEnv,
                    host = pairingHost,
                    code = pairingCode,
                    onHostChange = { pairingHost = it },
                    onCodeChange = { pairingCode = it },
                    onDone = { editing = null },
                    modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp),
                )
            }
        } else {
            PageHeader(
                title = "Connection",
                onBack = onBack,
                actions = {
                    HeaderIconButton(
                        icon = TablerIcons.Outline.Plus,
                        contentDescription = "Add environment",
                        onClick = { editing = "__create__" },
                    )
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
                        ActionRow(icon = TablerIcons.Outline.PlugConnected, title = "Disconnect backend", tint = NewTheme.TextPrimary) {
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

/**
 * The server card: where the server is, and the code that lets this phone
 * join it.
 *
 * Shared by the add and edit sheets so they read the same. The two fields
 * are what the server prints (`console pair`), so a user copies one line
 * and types two short things.
 *
 * Pasting the whole URL into the host field works too: anything after the
 * "#" is the code, so the app splits it into the two fields itself.
 */
@Composable
private fun ServerCard(
    host: String,
    code: String,
    onHostChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    SectionCard(modifier = modifier) {
        TextFieldRow(
            "Host",
            host,
            { onHostChange(it) },
            "192.168.1.100:8080",
            monospace = true,
        )
        SectionDivider(startInset = 18.dp)
        TextFieldRow(
            "Pairing code",
            code,
            { onCodeChange(it) },
            "abc-123-xyz",
            monospace = true,
        )
    }
}

/**
 * Add a server: its host, and the pairing code that proves this phone may
 * join it.
 */
@Composable
private fun AddEnvironmentForm(
    host: String,
    code: String,
    onHostChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    error: String?,
    onDone: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    var failure by remember { mutableStateOf<String?>(null) }

    Column(modifier = modifier) {
        ServerCard(
            host = host,
            code = code,
            onHostChange = { text ->
                // A pasted pairing URL carries the code in its fragment, so
                // the field splits it into the two.
                val (address, pasted) = splitHostAndCode(text)
                onHostChange(address)
                if (pasted.isNotBlank()) onCodeChange(pasted)
            },
            onCodeChange = onCodeChange,
        )
        (failure ?: error)?.let { message ->
            Text(message, color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        }
        ActionButton(
            text = if (busy) "Adding…" else "Add environment",
            onClick = {
                if (busy) return@ActionButton
                val normalized = normalizeBackendUrl(host) ?: run {
                    failure = "That host cannot be used."
                    return@ActionButton
                }
                busy = true
                failure = null
                scope.launch {
                    try {
                        // A code means the server wants this phone to pair:
                        // exchange it for a credential, and only then add
                        // the environment.
                        var serverName = ""
                        if (code.isNotBlank()) {
                            val paired = AppContainer.authRepository.pairWithCode(normalized, code, deviceName())
                            serverName = paired.server?.name?.takeIf { it.isNotBlank() }.orEmpty()
                        }
                        // The server names itself; without a pairing code
                        // there is nobody to ask, so the host stands in.
                        val label = serverName.ifBlank { urlHostPort(normalized) }
                        withContext(Dispatchers.IO) {
                            AppContainer.environmentsRepository.activateUrl(label, normalized)
                        }
                        onDone()
                    } catch (e: Exception) {
                        failure = e.message ?: "Could not add the server."
                    } finally {
                        busy = false
                    }
                }
            },
            enabled = !busy && host.isNotBlank(),
            loading = busy,
            kind = ActionButtonKind.Primary,
            modifier = Modifier.fillMaxWidth().padding(top = 20.dp),
        )
    }
}

/**
 * Edit an environment: its name, where it is, and the code that pairs this
 * phone with it.
 *
 * The layout matches the add sheet so the two read as one thing: a name
 * card, then the server card.
 */
@Composable
private fun EnvironmentEditorForm(
    env: Environment?,
    host: String,
    code: String,
    onHostChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    onDone: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val scope = rememberCoroutineScope()
    val envState by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    var name by remember(env?.id) { mutableStateOf(env?.name ?: "") }
    var status by remember(env?.id) { mutableStateOf("idle") } // idle|testing|test-ok|test-fail|saving
    var failure by remember(env?.id) { mutableStateOf<String?>(null) }
    val isActive = env != null && envState.activeId == env.id
    val canDelete = env != null && !(isActive && envState.environments.size == 1)

    Column(modifier = modifier) {
        SectionCard {
            TextFieldRow(
                "Name",
                name,
                { name = it; if (status != "testing" && status != "saving") status = "idle" },
                "My server",
            )
        }
        ServerCard(
            host = host,
            code = code,
            onHostChange = { text ->
                // A pasted pairing URL carries the code in its fragment, so
                // the field splits it into the two.
                val (address, pasted) = splitHostAndCode(text)
                onHostChange(address)
                if (pasted.isNotBlank()) onCodeChange(pasted)
                if (status != "testing" && status != "saving") status = "idle"
            },
            onCodeChange = { onCodeChange(it); if (status != "testing" && status != "saving") status = "idle" },
            modifier = Modifier.padding(top = 12.dp),
        )
        failure?.let { message ->
            Text(message, color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        }
        ActionButton(
            text = if (status == "testing") "Testing…" else "Test connection",
            onClick = {
                if (host.isBlank()) return@ActionButton
                status = "testing"
                scope.launch {
                    val ok = AppContainer.authRepository.probeServer(host)
                    status = if (ok) "test-ok" else "test-fail"
                }
            },
            enabled = host.isNotBlank() && status != "testing",
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
                val normalized = normalizeBackendUrl(host)
                if (normalized == null) {
                    confirmAlert("Invalid URL", "Backend server endpoint cannot be empty.")
                } else if (status != "test-ok") {
                    confirmAlert("Untested environment", "Save without a successful connection test?", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Save", onPress = {
                        scope.launch {
                            status = "saving"
                            try {
                                saveEnvironment(env, name, normalized, code)
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
                            saveEnvironment(env, name, normalized, code)
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
                enabled = host.isNotBlank() && status != "saving",
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

/**
 * Save an environment, pairing with it first when a code was given.
 *
 * The order matters: a device token is only valid on the machine that
 * issued it, so the credential is taken before the app starts talking to
 * the new address.
 */
private suspend fun saveEnvironment(env: Environment?, name: String, url: String, code: String) {
    withContext(Dispatchers.IO) {
        if (code.isNotBlank()) {
            AppContainer.authRepository.pairWithCode(url, code, deviceName())
        }
        if (env != null) {
            AppContainer.environmentsRepository.updateEnvironment(env.id, name.ifBlank { env.name }, url)
        } else {
            AppContainer.environmentsRepository.addEnvironment(name.ifBlank { "Default" }, url)
        }
    }
}
