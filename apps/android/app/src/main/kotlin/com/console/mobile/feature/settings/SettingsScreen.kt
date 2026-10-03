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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.BrandGithubCopilot
import io.github.lyxnx.compose.ui.tablericons.outline.ChartLine
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import io.github.lyxnx.compose.ui.tablericons.outline.UserCircle
import io.github.lyxnx.compose.ui.tablericons.outline.Wifi
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors

enum class SettingsSection { Servers, Providers, Usage, Models, Projects, DeletedChats, Mcp }

/**
 * Port of screens/settings/settings-screen.tsx.
 * Landing list → sub-screen. Back returns to list, then to home via onBackToHome.
 */
@Composable
fun SettingsScreen(onBackToHome: () -> Unit, onAddProject: () -> Unit) {
    var section by remember { mutableStateOf<SettingsSection?>(null) }
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()

    // Env switcher "Add" flow jumps straight into Servers.
    LaunchedEffect(appState.pendingServersSection) {
        if (appState.pendingServersSection) {
            section = SettingsSection.Servers
            AppContainer.appStateHolder.setPendingServersSection(false)
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        when (val s = section) {
            null -> SettingsLanding(onBack = onBackToHome, onOpen = { section = it })
            SettingsSection.Servers -> ServersSettings(onBack = { section = null })
            SettingsSection.Providers -> ProvidersSettings(onBack = { section = null })
            SettingsSection.Usage -> UsageSettings(onBack = { section = null })
            SettingsSection.Models -> ModelsSettings(onBack = { section = null })
            SettingsSection.Projects -> ProjectsSettings(onBack = { section = null }, onAddProject = onAddProject)
            SettingsSection.Mcp -> McpSettings(onBack = { section = null })
            SettingsSection.DeletedChats -> DeletedChatsSettings(onBack = { section = null })
        }
    }
}

@Composable
private fun SettingsLanding(onBack: () -> Unit, onOpen: (SettingsSection) -> Unit) {
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()

    // The landing row reports how many roles are configured, so the roles have
    // to be in state before the list renders.
    LaunchedEffect(Unit) { AppContainer.providerRepository.loadSettings() }

    ScreenHeader(title = "Settings", onBack = { onBack() })
    Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(top = 8.dp, bottom = 32.dp)) {
        val signedIn = authState.status?.values?.any { it.loggedIn } == true
        LandingRow(icon = TablerIcons.Outline.Wifi, title = "Servers", summary = if (!appState.backendUrl.isNullOrBlank()) "Connected" else "Not connected") { onOpen(SettingsSection.Servers) }
        LandingRow(icon = TablerIcons.Outline.UserCircle, title = "Providers", summary = if (signedIn) "Signed in" else "No providers connected") { onOpen(SettingsSection.Providers) }
        LandingRow(icon = TablerIcons.Outline.ChartLine, title = "Usage", summary = "Quota & limits") { onOpen(SettingsSection.Usage) }
        val roles = providerState.modelRoles.count { it.value.isNotBlank() }
        LandingRow(icon = TablerIcons.Outline.BrandGithubCopilot, title = "Models", summary = if (roles == 0) "Chat model only" else "$roles role${if (roles == 1) "" else "s"} configured") { onOpen(SettingsSection.Models) }
        val n = projectState.projects.size
        LandingRow(icon = TablerIcons.Outline.Folder, title = "Projects", summary = "$n project folder${if (n == 1) "" else "s"}") { onOpen(SettingsSection.Projects) }
        val d = projectState.deletedSessions.size
        LandingRow(icon = TablerIcons.Outline.Trash, title = "Deleted Chats", summary = "$d deleted chat${if (d == 1) "" else "s"}") { onOpen(SettingsSection.DeletedChats) }
        LandingRow(icon = TablerIcons.Outline.PlugConnected, title = "MCP Servers", summary = "External tools & services") { onOpen(SettingsSection.Mcp) }
    }
}

@Composable
private fun LandingRow(icon: ImageVector, title: String, summary: String, onClick: () -> Unit) {
    val shape = RoundedCornerShape(16.dp)
    Row(
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).clip(shape)
            .background(ConsoleColors.Card)
            .border(1.dp, ConsoleColors.Border, shape)
            .clickable(onClick = onClick)
            .padding(horizontal = 16.dp, vertical = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier.size(40.dp).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.SurfaceElevated),
            contentAlignment = Alignment.Center,
        ) {
            Icon(icon, contentDescription = null, tint = ConsoleColors.TextPrimary)
        }
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
            Text(title, color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
            Text(summary, color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 2.dp))
        }
        Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted)
    }
}
