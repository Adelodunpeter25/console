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
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.PillButtonVariant
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily

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
internal fun McpServerCard(
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
            McpBadge(text = server.transport.orEmpty(), tint = ConsoleColors.TextSecondary)
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
                        tool.description?.takeIf { it.isNotBlank() }?.let { desc ->
                            Text(desc, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(bottom = 4.dp))
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

