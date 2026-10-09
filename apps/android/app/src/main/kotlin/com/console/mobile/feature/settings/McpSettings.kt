package com.console.mobile.feature.settings

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.border
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
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Server
import com.console.mobile.AppContainer
import com.console.mobile.feature.settings.mcp.McpEditorForm
import com.console.mobile.feature.settings.mcp.McpOAuthLauncher
import com.console.mobile.feature.settings.mcp.McpServerCard
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.launch
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.AddButton
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight

/**
 * MCP servers settings, mirroring the desktop `mcp_page.rs`: server cards
 * with status badges and tools accordion, add/edit form, connect /
 * disconnect / delete, and browser OAuth via [McpOAuthLauncher].
 */
@Composable
fun McpSettings(onBack: () -> Unit) {
    val mcpState by AppContainer.mcpStateHolder.state.collectAsStateWithLifecycle()
    var editing by remember { mutableStateOf<String?>(null) } // null=list, "__create__"=create, else server id
    BackHandler(enabled = editing != null) { editing = null }
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

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        if (editing != null) {
            val serverId = editing!!.takeIf { it != "__create__" }
            val server = mcpState.servers.firstOrNull { it.id == serverId }
            PageHeader(title = if (serverId != null) "Edit MCP server" else "Add MCP server", onBack = { editing = null })
            McpEditorForm(server = server, onDone = { editing = null }, modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp))
        } else {
            Box(modifier = Modifier.fillMaxSize()) {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(rememberScrollState())
                        .padding(horizontal = 16.dp)
                        .padding(top = PageHeaderHeight, bottom = 40.dp),
                ) {
                    Text(
                        "Connect external tools and services via local stdio or remote HTTP servers.",
                        color = NewTheme.TextSecondary,
                        fontSize = 14.sp,
                        modifier = Modifier.padding(horizontal = 12.dp).padding(top = 8.dp),
                    )
                    mcpState.error?.let { Banner(it, modifier = Modifier.padding(top = 16.dp)) }
                    if (mcpState.loading && mcpState.servers.isEmpty()) {
                        LoadingState("Loading MCP servers…")
                    } else if (mcpState.servers.isEmpty()) {
                        McpEmptyState { editing = "__create__" }
                    } else {
                        Section("Servers") {
                            mcpState.servers.forEachIndexed { index, server ->
                                McpServerCard(
                                    server = server,
                                    busy = server.id in connectingIds || server.id in mcpState.busyServerIds,
                                    onConnect = { connect(server.id) },
                                    onDisconnect = { AppContainer.mcpRepository.disconnectServer(server.id) },
                                    onEdit = { editing = server.id },
                                    onDelete = { AppContainer.mcpRepository.deleteServer(server.id) },
                                )
                                if (index < mcpState.servers.lastIndex) SectionDivider()
                            }
                        }
                    }
                }
                PageHeader(
                    title = "MCP Servers",
                    blurred = true,
                    onBack = onBack,
                    actions = { AddButton("Add MCP server") { editing = "__create__" } },
                    modifier = Modifier.align(Alignment.TopCenter),
                )
            }
        }
    }
}

@Composable
private fun McpEmptyState(onAdd: () -> Unit) {
    EmptyView(
        title = "No MCP servers configured",
        description = "Add local stdio commands or remote HTTP servers to equip the harness with dynamic tools.",
        icon = TablerIcons.Outline.Server, iconTint = NewTheme.TextMuted,
        action = {
            ActionButton(text = "Add your first MCP server", onClick = onAdd, kind = ActionButtonKind.Primary)
        },
    )
}
