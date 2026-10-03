package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Server
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import com.console.mobile.AppContainer
import com.console.mobile.data.model.McpAuthConfig
import com.console.mobile.data.model.McpSavePayload
import com.console.mobile.data.model.McpServerEntry
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.PillButtonVariant
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.launch

/**
 * MCP servers settings, mirroring the desktop `mcp_page.rs`: server cards
 * with status badges and tools accordion, add/edit form, connect /
 * disconnect / delete, and browser OAuth via [McpOAuthLauncher].
 */
@Composable
fun McpSettings(onBack: () -> Unit) {
    val mcpState by AppContainer.mcpStateHolder.state.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<String?>(null) } // null=list, "__create__"=create, else server id
    var connectingIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    LaunchedEffect(Unit) { AppContainer.mcpRepository.loadServers() }

    fun connect(id: String) {
        if (id in connectingIds) return
        connectingIds = connectingIds + id
        scope.launch {
            try {
                McpOAuthLauncher.connectAndAuthorize(context, AppContainer.mcpRepository, id)
            } finally {
                connectingIds = connectingIds - id
                AppContainer.mcpRepository.loadServers()
            }
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        if (editing != null) {
            val serverId = editing!!.takeIf { it != "__create__" }
            val server = mcpState.servers.firstOrNull { it.id == serverId }
            ScreenHeader(title = if (serverId != null) "Edit MCP server" else "Add MCP server", onBack = { editing = null })
            McpEditorForm(server = server, onDone = { editing = null }, modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp))
        } else {
            ScreenHeader(
                title = "MCP Servers",
                onBack = onBack,
                actions = {
                    IconButton(onClick = { editing = "__create__" }, modifier = Modifier.size(40.dp)) {
                        Box(
                            modifier = Modifier.size(40.dp).clip(androidx.compose.foundation.shape.CircleShape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, androidx.compose.foundation.shape.CircleShape),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(TablerIcons.Outline.Plus, contentDescription = "Add MCP server", tint = ConsoleColors.TextPrimary)
                        }
                    }
                },
            )
            Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
                Text(
                    "Connect external tools and services via local stdio or remote HTTP servers.",
                    color = ConsoleColors.TextSecondary,
                    fontSize = 14.sp,
                    modifier = Modifier.padding(horizontal = 4.dp).padding(top = 8.dp, bottom = 16.dp),
                )
                if (mcpState.error != null) {
                    Row(
                        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp))
                            .background(ConsoleColors.Destructive.copy(alpha = 0.08f))
                            .border(1.dp, ConsoleColors.Destructive.copy(alpha = 0.3f), RoundedCornerShape(12.dp))
                            .padding(horizontal = 12.dp, vertical = 10.dp)
                            .padding(bottom = 0.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(16.dp))
                        Text(mcpState.error!!, color = ConsoleColors.Destructive, fontSize = 12.sp, modifier = Modifier.padding(start = 8.dp))
                    }
                    androidx.compose.foundation.layout.Spacer(modifier = Modifier.padding(top = 12.dp))
                }
                if (mcpState.loading && mcpState.servers.isEmpty()) {
                    Box(modifier = Modifier.fillMaxWidth().padding(top = 48.dp), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                            Text("Loading MCP servers…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                        }
                    }
                } else if (mcpState.servers.isEmpty()) {
                    McpEmptyState { editing = "__create__" }
                } else {
                    mcpState.servers.forEach { server ->
                        McpServerCard(
                            server = server,
                            busy = server.id in connectingIds || server.id in mcpState.busyServerIds,
                            onConnect = { connect(server.id) },
                            onDisconnect = { AppContainer.mcpRepository.disconnectServer(server.id) },
                            onEdit = { editing = server.id },
                            onDelete = { AppContainer.mcpRepository.deleteServer(server.id) },
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun McpEmptyState(onAdd: () -> Unit) {
    val shape = RoundedCornerShape(16.dp)
    Column(
        modifier = Modifier.fillMaxWidth().clip(shape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, shape).padding(horizontal = 20.dp, vertical = 32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Icon(TablerIcons.Outline.Server, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(36.dp))
        Text("No MCP servers configured", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(top = 12.dp))
        Text(
            "Add local stdio commands or remote HTTP servers to equip the harness with dynamic tools.",
            color = ConsoleColors.TextSecondary,
            fontSize = 12.sp,
            modifier = Modifier.padding(top = 6.dp, bottom = 16.dp),
        )
        PillButton(text = "Add your first MCP server", onClick = onAdd)
    }
}

private fun mcpStatusColor(status: String): Color = when (status) {
    "connected" -> Color(0xFF4ADE80)
    "connecting" -> Color(0xFF60A5FA)
    "needs_auth" -> Color(0xFFFBBF24)
    "error" -> ConsoleColors.Destructive
    else -> ConsoleColors.TextSecondary
}

private fun mcpStatusLabel(server: McpServerEntry): String = when (server.status) {
    "connected" -> "Connected (${server.tools.size} tools)"
    "connecting" -> "Connecting…"
    "needs_auth" -> "Needs Auth"
    "error" -> "Error: ${server.error ?: "connect failed"}"
    else -> "Disconnected"
}

@Composable
private fun McpServerCard(
    server: McpServerEntry,
    busy: Boolean,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var expanded by remember(server.id) { mutableStateOf(false) }
    val shape = RoundedCornerShape(16.dp)
    val statusColor = mcpStatusColor(server.status)
    val target = when (server.transport) {
        "http" -> server.url.orEmpty()
        else -> listOfNotNull(server.command, server.args.joinToString(" ").ifBlank { null }).joinToString(" ")
    }

    Column(
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).clip(shape)
            .background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, shape).padding(14.dp),
    ) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Text(
                server.displayName,
                color = ConsoleColors.TextPrimary,
                fontSize = 14.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            McpBadge(text = server.transport, tint = ConsoleColors.TextSecondary)
            McpBadge(text = mcpStatusLabel(server), tint = statusColor)
        }
        if (target.isNotBlank()) {
            Text(
                target,
                color = ConsoleColors.TextSecondary,
                fontSize = 12.sp,
                fontFamily = ConsoleMonoFamily,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(top = 6.dp),
            )
        }
        if (server.tools.isNotEmpty()) {
            Row(
                modifier = Modifier.fillMaxWidth().clickable { expanded = !expanded }.padding(top = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    if (expanded) TablerIcons.Outline.ChevronDown else TablerIcons.Outline.ChevronRight,
                    contentDescription = null,
                    tint = ConsoleColors.TextSecondary,
                    modifier = Modifier.size(14.dp),
                )
                Text("${server.tools.size} Advertised Tools", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 4.dp))
            }
            if (expanded) {
                Column(
                    modifier = Modifier.fillMaxWidth().padding(top = 6.dp).clip(RoundedCornerShape(8.dp))
                        .background(ConsoleColors.Background).padding(8.dp),
                ) {
                    server.tools.forEach { tool ->
                        Text(tool.name, color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (tool.description.isNotBlank()) {
                            Text(tool.description, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(bottom = 4.dp))
                        }
                    }
                }
            }
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 10.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            val connectLabel = when {
                server.needsAuth -> "Authorize"
                server.isConnected -> "Reconnect"
                else -> "Connect"
            }
            PillButton(
                text = connectLabel,
                onClick = onConnect,
                enabled = !busy,
                loading = busy,
                icon = TablerIcons.Outline.PlugConnected,
                variant = PillButtonVariant.Outline,
                cornerRadius = 8.dp,
                horizontalPadding = 10.dp,
                verticalPadding = 5.dp,
            )
            if (server.isConnected) {
                PillButton(
                    text = "Disconnect",
                    onClick = onDisconnect,
                    enabled = !busy,
                    variant = PillButtonVariant.Outline,
                    cornerRadius = 8.dp,
                    horizontalPadding = 10.dp,
                    verticalPadding = 5.dp,
                )
            }
            PillButton(
                text = "Edit",
                onClick = onEdit,
                enabled = !busy,
                variant = PillButtonVariant.Outline,
                cornerRadius = 8.dp,
                horizontalPadding = 10.dp,
                verticalPadding = 5.dp,
            )
            PillButton(
                text = "Delete",
                onClick = {
                    confirmAlert(
                        title = "Delete MCP server?",
                        message = "“${server.displayName}” will be removed. This cannot be undone.",
                        buttons = listOf(
                            ConfirmButton("Cancel", cancel = true),
                            ConfirmButton("Delete", destructive = true, onPress = onDelete),
                        ),
                    )
                },
                enabled = !busy,
                variant = PillButtonVariant.Destructive,
                cornerRadius = 8.dp,
                horizontalPadding = 10.dp,
                verticalPadding = 5.dp,
                icon = TablerIcons.Outline.Trash,
            )
        }
    }
}

@Composable
private fun McpBadge(text: String, tint: Color) {
    Text(
        text,
        color = tint,
        fontSize = 10.sp,
        fontWeight = FontWeight.Medium,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier.padding(start = 6.dp).clip(RoundedCornerShape(4.dp)).background(tint.copy(alpha = 0.12f)).padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

@Composable
private fun McpEditorForm(server: McpServerEntry?, onDone: () -> Unit, modifier: Modifier = Modifier) {
    var name by remember(server?.id) { mutableStateOf(server?.displayName ?: "") }
    var isHttp by remember(server?.id) { mutableStateOf(server?.transport == "http") }
    var url by remember(server?.id) { mutableStateOf(server?.url ?: "") }
    var command by remember(server?.id) { mutableStateOf(server?.command ?: "") }
    var args by remember(server?.id) { mutableStateOf(server?.args?.joinToString(" ") ?: "") }
    var env by remember(server?.id) {
        mutableStateOf(server?.env?.entries?.joinToString(", ") { (k, v) -> "$k=$v" } ?: "")
    }
    var auth by remember(server?.id) { mutableStateOf(server?.auth?.type ?: "none") }
    var token by remember(server?.id) { mutableStateOf("") }
    var saving by remember { mutableStateOf(false) }
    var formError by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    val fieldColors = OutlinedTextFieldDefaults.colors(
        focusedBorderColor = ConsoleColors.Border,
        unfocusedBorderColor = ConsoleColors.Border,
        focusedTextColor = ConsoleColors.TextPrimary,
        unfocusedTextColor = ConsoleColors.TextPrimary,
        cursorColor = ConsoleColors.TextPrimary,
    )
    @Composable
    fun Field(label: String, value: String, onValue: (String) -> Unit, placeholder: String, singleLine: Boolean = true) {
        Text(label, color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
        OutlinedTextField(
            value = value,
            onValueChange = onValue,
            placeholder = { Text(placeholder, color = ConsoleColors.TextMuted, fontSize = 13.sp) },
            singleLine = singleLine,
            colors = fieldColors,
            shape = RoundedCornerShape(12.dp),
            modifier = Modifier.fillMaxWidth(),
        )
    }

    fun save() {
        if (name.isBlank()) {
            formError = "Server name is required"
            return
        }
        if (isHttp && url.isBlank()) {
            formError = "Endpoint URL is required"
            return
        }
        if (!isHttp && command.isBlank()) {
            formError = "Command is required"
            return
        }
        if (isHttp && auth == "static" && token.isBlank() && server == null) {
            formError = "Token is required for static auth"
            return
        }
        val id = server?.id ?: name.trim().lowercase().replace(' ', '-').filter { it.isLetterOrDigit() || it == '-' }
        if (id.isBlank()) {
            formError = "Server name must contain letters or digits"
            return
        }
        val payload = com.console.mobile.data.model.McpSavePayload(
            label = name.trim(),
            transport = if (isHttp) "http" else "stdio",
            url = url.trim().ifBlank { null }?.takeIf { isHttp },
            command = command.trim().ifBlank { null }?.takeIf { !isHttp },
            args = args.split(Regex("\\s+")).filter { it.isNotBlank() }.takeIf { !isHttp } ?: emptyList(),
            env = env.split(',').mapNotNull { part ->
                val k = part.substringBefore('=').trim()
                val v = part.substringAfter('=', "").trim()
                if (k.isEmpty()) null else k to v
            }.toMap().takeIf { !isHttp } ?: emptyMap(),
            auth = com.console.mobile.data.model.McpAuthConfig(type = auth),
            token = token.ifBlank { null }?.takeIf { isHttp && auth == "static" },
            enabled = true,
        )
        saving = true
        formError = null
        scope.launch {
            var ok = false
            try {
                if (server != null) {
                    AppContainer.mcpRepository.saveServer(payload, server.id) { ok = it }
                } else {
                    AppContainer.mcpRepository.saveServer(payload.copy(id = id)) { ok = it }
                }
            } finally {
                saving = false
                if (ok) onDone()
            }
        }
    }

    Column(modifier = modifier) {
        Field("Server Name / ID", name, { name = it }, "e.g. atlassian, filesystem")
        Text("Transport Type", color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            PillButton(
                text = "Local (stdio)",
                onClick = { isHttp = false },
                variant = if (!isHttp) PillButtonVariant.Filled else PillButtonVariant.Outline,
                cornerRadius = 8.dp,
            )
            PillButton(
                text = "Remote (HTTP)",
                onClick = { isHttp = true },
                variant = if (isHttp) PillButtonVariant.Filled else PillButtonVariant.Outline,
                cornerRadius = 8.dp,
            )
        }
        if (isHttp) {
            Field("Endpoint URL", url, { url = it }, "https://mcp.atlassian.com/v2/mcp")
            Text("Authentication", color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                PillButton(text = "OAuth 2.1", onClick = { auth = "oauth2" }, variant = if (auth == "oauth2") PillButtonVariant.Filled else PillButtonVariant.Outline, cornerRadius = 8.dp)
                PillButton(text = "Static Token", onClick = { auth = "static" }, variant = if (auth == "static") PillButtonVariant.Filled else PillButtonVariant.Outline, cornerRadius = 8.dp)
                PillButton(text = "None", onClick = { auth = "none" }, variant = if (auth == "none") PillButtonVariant.Filled else PillButtonVariant.Outline, cornerRadius = 8.dp)
            }
            if (auth == "static") {
                Field("Token", token, { token = it }, if (server != null) "Leave blank to keep the stored token" else "Bearer token (stored server-side, never shown again)")
            }
        } else {
            Field("Command", command, { command = it }, "npx, uvx, docker, or path to binary")
            Field("Arguments (space-delimited)", args, { args = it }, "-y @modelcontextprotocol/server-filesystem /path/to/folder")
            Field("Environment Variables (KEY=VAL, comma-separated)", env, { env = it }, "API_KEY=xyz, DEBUG=true")
        }
        if (formError != null) {
            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 12.dp).clip(RoundedCornerShape(12.dp))
                    .background(ConsoleColors.Destructive.copy(alpha = 0.08f))
                    .border(1.dp, ConsoleColors.Destructive.copy(alpha = 0.3f), RoundedCornerShape(12.dp))
                    .padding(horizontal = 12.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(16.dp))
                Text(formError!!, color = ConsoleColors.Destructive, fontSize = 12.sp, modifier = Modifier.padding(start = 8.dp))
            }
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            PillButton(text = "Cancel", onClick = onDone, enabled = !saving, variant = PillButtonVariant.Outline, cornerRadius = 8.dp, modifier = Modifier.weight(1f), fullWidth = true)
            PillButton(
                text = if (saving) "Saving…" else "Save Server",
                onClick = ::save,
                enabled = !saving,
                cornerRadius = 8.dp,
                modifier = Modifier.weight(1f),
                fullWidth = true,
            )
        }
        if (server != null) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 12.dp)) {
                Icon(TablerIcons.Outline.Check, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(14.dp))
                Text(
                    "Editing “${server.displayName}”. Token fields left blank keep their stored value.",
                    color = ConsoleColors.TextSecondary,
                    fontSize = 11.sp,
                    modifier = Modifier.padding(start = 6.dp),
                )
            }
        }
    }
}
