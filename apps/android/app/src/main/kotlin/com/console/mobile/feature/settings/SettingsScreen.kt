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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Wifi
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
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors

enum class SettingsSection { Connection, Account, Usage, Projects, DeletedChats }

/**
 * Port of screens/settings/settings-screen.tsx.
 * Landing list → sub-screen. Back returns to list, then to home via onBackToHome.
 */
@Composable
fun SettingsScreen(onBackToHome: () -> Unit) {
    var section by remember { mutableStateOf<SettingsSection?>(null) }
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()

    // Env switcher "Add" flow jumps straight into Connection.
    LaunchedEffect(appState.pendingConnectionSection) {
        if (appState.pendingConnectionSection) {
            section = SettingsSection.Connection
            AppContainer.appStateHolder.setPendingConnectionSection(false)
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        when (val s = section) {
            null -> SettingsLanding(onBack = onBackToHome, onOpen = { section = it })
            SettingsSection.Connection -> ConnectionSettings(onBack = { section = null })
            SettingsSection.Account -> AccountSettings(onBack = { section = null })
            SettingsSection.Usage -> UsageSettings(onBack = { section = null })
            SettingsSection.Projects -> ProjectsSettings(onBack = { section = null })
            SettingsSection.DeletedChats -> DeletedChatsSettings(onBack = { section = null })
        }
    }
}

@Composable
private fun SettingsLanding(onBack: () -> Unit, onOpen: (SettingsSection) -> Unit) {
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()

    ScreenHeader(title = "Settings", onBack = { onBack() })
    Column(modifier = Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(top = 8.dp, bottom = 32.dp)) {
        val signedIn = authState.status?.values?.any { it.loggedIn } == true
        LandingRow(icon = Icons.Filled.Wifi, title = "Connection", summary = if (!appState.backendUrl.isNullOrBlank()) "Connected" else "Not connected") { onOpen(SettingsSection.Connection) }
        LandingRow(icon = Icons.Filled.Person, title = "Account", summary = if (signedIn) "Signed in" else "No providers connected") { onOpen(SettingsSection.Account) }
        LandingRow(icon = Icons.Filled.BarChart, title = "Usage", summary = "Quota & limits") { onOpen(SettingsSection.Usage) }
        val n = projectState.projects.size
        LandingRow(icon = Icons.Filled.Folder, title = "Projects", summary = "$n project folder${if (n == 1) "" else "s"}") { onOpen(SettingsSection.Projects) }
        val d = projectState.deletedSessions.size
        LandingRow(icon = Icons.Filled.Delete, title = "Deleted Chats", summary = "$d deleted chat${if (d == 1) "" else "s"}") { onOpen(SettingsSection.DeletedChats) }
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
        Icon(Icons.Filled.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted)
    }
}
