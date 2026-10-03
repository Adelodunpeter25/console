package com.console.mobile.feature.settings.mcp

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import com.console.mobile.AppContainer
import com.console.mobile.data.model.McpAuthConfig
import com.console.mobile.data.model.McpSavePayload
import com.console.mobile.data.model.McpServerEntry
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.PillButtonVariant
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.launch

@Composable
internal fun McpEditorForm(server: McpServerEntry?, onDone: () -> Unit, modifier: Modifier = Modifier) {
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
