package com.console.mobile.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.sp
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import com.console.mobile.feature.changes.ChangesPlaceholderScreen
import com.console.mobile.feature.chat.ChatPlaceholderScreen
import com.console.mobile.feature.files.FilesPlaceholderScreen
import com.console.mobile.feature.home.HomePlaceholderScreen
import com.console.mobile.feature.onboarding.OnboardingPlaceholderScreen
import com.console.mobile.feature.settings.SettingsPlaceholderScreen
import com.console.mobile.feature.subagents.SubagentDetailsPlaceholderScreen
import com.console.mobile.feature.subagents.SubagentsPlaceholderScreen
import com.console.mobile.feature.terminal.TerminalPlaceholderScreen
import com.console.mobile.ui.theme.ConsoleColors

private data class Tab(
    val label: String,
    val route: Any,
)

@Composable
fun AppNavGraph() {
    val navController = rememberNavController()
    var showOnboarding by remember { mutableStateOf(false) }

    if (showOnboarding) {
        OnboardingPlaceholderScreen(onConnected = { showOnboarding = false })
        return
    }

    val tabs = remember {
        listOf(
            Tab("Home", RouteHome),
            Tab("Chat", RouteChat),
            Tab("Files", RouteFiles),
            Tab("Changes", RouteChanges),
            Tab("Terminal", RouteTerminal),
        )
    }
    var selectedIndex by remember { mutableStateOf(0) }

    Scaffold(
        containerColor = ConsoleColors.Background,
        bottomBar = {
            NavigationBar(containerColor = ConsoleColors.Surface) {
                tabs.forEachIndexed { index, tab ->
                    NavigationBarItem(
                        selected = selectedIndex == index,
                        onClick = {
                            selectedIndex = index
                            val route = tab.route
                            navController.navigate(route) {
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        label = { Text(tab.label, fontSize = 10.sp) },
                        icon = {},
                    )
                }
            }
        },
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = RouteHome,
            modifier = Modifier.padding(padding),
        ) {
            composable<RouteHome> { HomePlaceholderScreen(onOpenSettings = { navController.navigate(RouteSettings) }, onOpenSubagents = { navController.navigate(RouteSubagents) }) }
            composable<RouteChat> { ChatPlaceholderScreen() }
            composable<RouteFiles> { FilesPlaceholderScreen() }
            composable<RouteChanges> { ChangesPlaceholderScreen() }
            composable<RouteTerminal> { TerminalPlaceholderScreen() }
            composable<RouteSettings> { SettingsPlaceholderScreen(onBack = { navController.popBackStack() }) }
            composable<RouteSubagents> { SubagentsPlaceholderScreen(onOpenDetails = { id -> navController.navigate(RouteSubagentDetails(id)) }) }
            composable<RouteSubagentDetails> { SubagentDetailsPlaceholderScreen() }
            composable<RouteOnboarding> { OnboardingPlaceholderScreen(onConnected = { showOnboarding = false }) }
        }
    }
}

@Composable
fun PlaceholderScreen(label: String) {
    Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        Text(label, color = ConsoleColors.TextSecondary)
    }
}
