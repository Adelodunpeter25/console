package com.console.mobile.feature.settings.mcp

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import console.v1.McpServerStatus as McpServerEntry
import com.console.mobile.data.model.displayName
import com.console.mobile.data.model.needsAuth
import com.console.mobile.data.model.isConnected
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind

private fun mcpStatusColor(status: String): Color = when (status) {
    "connected" -> NewTheme.Success
    "connecting" -> NewTheme.Gauge
    "needs_auth" -> NewTheme.Warning
    "error" -> NewTheme.Danger
    else -> NewTheme.TextMuted
}

private fun mcpStatusLabel(server: McpServerEntry): String = when (server.status) {
    "connected" -> "Connected · ${server.tools.size} tools"
    "connecting" -> "Connecting…"
    "needs_auth" -> "Needs authorization"
    "error" -> "Error: ${server.error ?: "connect failed"}"
    else -> "Disconnected"
}

/** One server as a row inside the Servers card: name and status on top, actions underneath. */
@Composable
internal fun McpServerCard(
    server: McpServerEntry,
    busy: Boolean,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    var expanded by remember(server.id) { mutableStateOf(false) }
    val target = when (server.transport) {
        "http" -> server.url.orEmpty()
        else -> listOfNotNull(server.command, server.args.joinToString(" ").ifBlank { null }).joinToString(" ")
    }

    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp)) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text(server.displayName, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(mcpStatusLabel(server), color = mcpStatusColor(server.status), fontSize = 13.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
            }
            if (!server.transport.isNullOrBlank()) {
                Text(
                    server.transport.orEmpty(), color = NewTheme.TextSecondary, fontSize = 11.sp, fontWeight = FontWeight.Medium,
                    modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(6.dp)).background(Color.White.copy(alpha = 0.07f)).padding(horizontal = 8.dp, vertical = 3.dp),
                )
            }
        }
        if (target.isNotBlank()) {
            Text(target, color = NewTheme.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 6.dp))
        }
        if (server.tools.isNotEmpty()) {
            Row(
                modifier = Modifier.fillMaxWidth().clickable { expanded = !expanded }.padding(top = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    if (expanded) TablerIcons.Outline.ChevronDown else TablerIcons.Outline.ChevronRight,
                    contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(16.dp),
                )
                Text("${server.tools.size} advertised tools", color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(start = 4.dp))
            }
            if (expanded) {
                Column(
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp).clip(RoundedCornerShape(NewTheme.ChipRadius))
                        .background(NewTheme.Background).padding(12.dp),
                ) {
                    server.tools.forEach { tool ->
                        Text(tool.name, color = NewTheme.TextPrimary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        tool.description?.takeIf { it.isNotBlank() }?.let { desc ->
                            Text(desc, color = NewTheme.TextSecondary, fontSize = 11.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(bottom = 6.dp))
                        }
                    }
                }
            }
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 12.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            val connectLabel = when {
                server.needsAuth -> "Authorize"
                server.isConnected -> "Reconnect"
                else -> "Connect"
            }
            ActionButton(connectLabel, onConnect, enabled = !busy, loading = busy, icon = TablerIcons.Outline.PlugConnected, compact = true)
            if (server.isConnected) ActionButton("Disconnect", onDisconnect, enabled = !busy, compact = true)
            ActionButton("Edit", onEdit, enabled = !busy, compact = true)
            ActionButton(
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
                kind = ActionButtonKind.Danger,
                enabled = !busy,
                icon = TablerIcons.Outline.Trash,
                compact = true,
            )
        }
    }
}
