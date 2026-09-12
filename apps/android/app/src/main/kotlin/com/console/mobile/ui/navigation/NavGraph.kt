package com.console.mobile.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Chat
import androidx.compose.material.icons.filled.ChangeCircle
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.console.mobile.AppContainer
import com.console.mobile.feature.changes.ChangesScreen
import com.console.mobile.feature.chat.ChatScreen
import com.console.mobile.feature.files.FilesScreen
import com.console.mobile.feature.home.HomeScreen
import com.console.mobile.feature.onboarding.OnboardingScreen
import com.console.mobile.feature.settings.SettingsScreen
import com.console.mobile.feature.subagents.SubagentDetailsScreen
import com.console.mobile.feature.subagents.SubagentsScreen
import com.console.mobile.feature.terminal.TerminalScreen
import com.console.mobile.ui.components.ConfirmDialogHost
import com.console.mobile.ui.theme.ConsoleColors

private data class Tab(
    val label: String,
    val route: Any,
    val icon: ImageVector,
)

@Composable
fun AppNavGraph() {
    val navController = rememberNavController()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    var booted by remember { mutableStateOf(false) }

    // EnvironmentsRepository init already applied the persisted active URL.
    LaunchedEffect(Unit) { booted = true }

    if (!booted) {
        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(color = ConsoleColors.TextPrimary)
        }
        return
    }

    if (appState.backendUrl.isNullOrBlank()) {
        OnboardingScreen(onConnected = {})
        ConfirmDialogHost()
        return
    }

    val tabs = remember {
        listOf(
            Tab("Home", RouteHome, Icons.Filled.Home),
            Tab("Chat", RouteChat, Icons.AutoMirrored.Filled.Chat),
            Tab("Files", RouteFiles, Icons.Filled.Folder),
            Tab("Changes", RouteChanges, Icons.Filled.ChangeCircle),
            Tab("Terminal", RouteTerminal, Icons.Filled.Terminal),
        )
    }

    val backStackEntry by navController.currentBackStackEntryAsState()
    val selectedIndex = remember(backStackEntry) {
        val dest = backStackEntry?.destination?.route ?: ""
        tabs.indexOfFirst { tab ->
            dest.contains(tab.route.javaClass.simpleName.removePrefix("Route"), ignoreCase = true)
        }.takeIf { it >= 0 } ?: 0
    }
    var pendingChatNav by remember { mutableStateOf<String?>(null) }

    // Deep-link from notification → open chat for that session.
    LaunchedEffect(Unit) {
        val pending = AppContainer.consumePendingOpenFromNotification()
        if (pending != null) pendingChatNav = pending
    }
    LaunchedEffect(pendingChatNav) {
        val id = pendingChatNav ?: return@LaunchedEffect
        AppContainer.appStateHolder.openChatSession(id)
        AppContainer.sessionRepository.loadDetail(id)
        pendingChatNav = null
        navController.navigate(RouteChat) { launchSingleTop = true }
    }

    // Keep nav in sync when Home opens a chat / subagent details programmatically.
    LaunchedEffect(appState.activeTab, appState.selectedSessionId) {
        if (appState.activeTab.name == "Chat") {
            val current = backStackEntry?.destination?.route ?: ""
            if (!current.contains("RouteChat", ignoreCase = true)) {
                navController.navigate(RouteChat) { launchSingleTop = true }
            }
        }
    }

    Scaffold(
        containerColor = ConsoleColors.Background,
        bottomBar = {
            NavigationBar(containerColor = ConsoleColors.Surface) {
                tabs.forEachIndexed { index, tab ->
                    NavigationBarItem(
                        selected = selectedIndex == index,
                        onClick = {
                            navController.navigate(tab.route) {
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        label = { Text(tab.label, fontSize = 10.sp) },
                        icon = { Icon(tab.icon, contentDescription = tab.label) },
                        colors = NavigationBarItemDefaults.colors(
                            selectedIconColor = ConsoleColors.TextPrimary,
                            selectedTextColor = ConsoleColors.TextPrimary,
                            unselectedIconColor = ConsoleColors.TextMuted,
                            unselectedTextColor = ConsoleColors.TextMuted,
                            indicatorColor = ConsoleColors.SurfaceElevated,
                        ),
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
            composable<RouteHome> {
                HomeScreen(
                    onOpenSettings = { navController.navigate(RouteSettings) },
                    onOpenChat = { navController.navigate(RouteChat) { launchSingleTop = true } },
                )
            }
            composable<RouteChat> {
                ChatScreen(
                    onBackToHome = {
                        AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
                        navController.navigate(RouteHome) { launchSingleTop = true }
                    },
                    onOpenTab = { tab ->
                        when (tab) {
                            com.console.mobile.data.store.MobileTab.Files -> navController.navigate(RouteFiles) { launchSingleTop = true }
                            com.console.mobile.data.store.MobileTab.Changes -> navController.navigate(RouteChanges) { launchSingleTop = true }
                            com.console.mobile.data.store.MobileTab.Terminal -> navController.navigate(RouteTerminal) { launchSingleTop = true }
                            com.console.mobile.data.store.MobileTab.Subagents -> navController.navigate(RouteSubagents) { launchSingleTop = true }
                            else -> {}
                        }
                    },
                    onOpenSubagentDetails = { id -> navController.navigate(RouteSubagentDetails(id)) },
                )
            }
            composable<RouteFiles> {
                FilesScreen(onBack = {
                    val prev = AppContainer.appStateHolder.state.value.previousTab
                    val target = if (prev != null && prev != com.console.mobile.data.store.MobileTab.Files) prev else com.console.mobile.data.store.MobileTab.Home
                    AppContainer.appStateHolder.setActiveTab(target)
                    when (target) {
                        com.console.mobile.data.store.MobileTab.Home -> navController.navigate(RouteHome) { launchSingleTop = true }
                        com.console.mobile.data.store.MobileTab.Chat -> navController.navigate(RouteChat) { launchSingleTop = true }
                        else -> navController.navigate(RouteHome) { launchSingleTop = true }
                    }
                })
            }
            composable<RouteChanges> {
                ChangesScreen(onBack = {
                    val prev = AppContainer.appStateHolder.state.value.previousTab
                    val target = if (prev != null && prev != com.console.mobile.data.store.MobileTab.Changes) prev else com.console.mobile.data.store.MobileTab.Chat
                    AppContainer.appStateHolder.setActiveTab(target)
                    when (target) {
                        com.console.mobile.data.store.MobileTab.Home -> navController.navigate(RouteHome) { launchSingleTop = true }
                        com.console.mobile.data.store.MobileTab.Chat -> navController.navigate(RouteChat) { launchSingleTop = true }
                        else -> navController.navigate(RouteChat) { launchSingleTop = true }
                    }
                })
            }
            composable<RouteTerminal> {
                TerminalScreen(onBack = {
                    val prev = AppContainer.appStateHolder.state.value.previousTab
                    val target = if (prev != null && prev != com.console.mobile.data.store.MobileTab.Terminal) prev else com.console.mobile.data.store.MobileTab.Home
                    AppContainer.appStateHolder.setActiveTab(target)
                    when (target) {
                        com.console.mobile.data.store.MobileTab.Home -> navController.navigate(RouteHome) { launchSingleTop = true }
                        com.console.mobile.data.store.MobileTab.Chat -> navController.navigate(RouteChat) { launchSingleTop = true }
                        else -> navController.navigate(RouteHome) { launchSingleTop = true }
                    }
                })
            }
            composable<RouteSettings> {
                SettingsScreen(onBackToHome = {
                    AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
                    navController.navigate(RouteHome) {
                        launchSingleTop = true
                        popUpTo(RouteHome)
                    }
                })
            }
            composable<RouteSubagents> {
                SubagentsScreen(
                    onBackToChat = {
                        AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Chat)
                        navController.navigate(RouteChat) { launchSingleTop = true }
                    },
                    onOpenDetails = { id -> navController.navigate(RouteSubagentDetails(id)) },
                )
            }
            composable<RouteSubagentDetails> {
                SubagentDetailsScreen(onBack = {
                    AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Subagents)
                    navController.popBackStack()
                })
            }
            composable<RouteOnboarding> { OnboardingScreen(onConnected = {}) }
        }
        ConfirmDialogHost()
    }
}

@Composable
fun PlaceholderScreen(label: String) {
    Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Text(label, color = ConsoleColors.TextSecondary)
    }
}
