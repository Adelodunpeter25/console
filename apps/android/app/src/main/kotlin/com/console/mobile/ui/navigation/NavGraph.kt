package com.console.mobile.ui.navigation

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import com.console.mobile.AppContainer
import com.console.mobile.core.notification.NotificationPermissionRequest
import com.console.mobile.feature.changes.ChangesScreen
import com.console.mobile.feature.chat.ChatScreen
import com.console.mobile.feature.devices.DevicesScreen
import com.console.mobile.feature.files.FilesScreen
import com.console.mobile.feature.home.HomeScreen
import com.console.mobile.feature.onboarding.OnboardingScreen
import com.console.mobile.feature.onboarding.SetupScreen
import com.console.mobile.feature.onboarding.SetupStep
import com.console.mobile.feature.settings.AddProjectScreen
import com.console.mobile.feature.settings.SettingsScreen
import com.console.mobile.feature.subagents.SubagentDetailsScreen
import com.console.mobile.feature.subagents.SubagentsScreen
import com.console.mobile.feature.terminal.TerminalScreen
import com.console.mobile.ui.components.common.new.ConfirmPromptHost
import com.console.mobile.ui.theme.NewTheme

@Composable
fun AppNavGraph() {
    val navController = rememberNavController()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()

    if (appState.backendUrl.isNullOrBlank()) {
        OnboardingScreen(onConnected = {})
        ConfirmPromptHost()
        return
    }

    // Setup is a real route rather than an early return. Returning before the
    // NavHost composes meant the panel could not navigate anywhere — its
    // buttons would fire a navigation into a graph that did not exist yet.
    //
    // Re-keyed on the active environment, so switching to a newly added server
    // runs the check again for that server alone.
    val setupState by AppContainer.setupStateHolder.state.collectAsStateWithLifecycle()
    val environments by AppContainer.environmentsStateHolder.state.collectAsStateWithLifecycle()
    LaunchedEffect(environments.activeId, appState.backendUrl) {
        AppContainer.setupRepository.maybeShow(environments.activeId)
    }
    // Setup appears as the start destination when the app opens onto a server
    // that has not been through it, so there is no flash of Home first.
    //
    // `remember` pins that decision to the first composition on purpose. An
    // *unpinned* start destination rebuilds the NavHost graph whenever the value
    // changes, and rebuilding the graph clears the back stack — which is exactly
    // why stepping out to Providers used to leave back with nowhere to go. The
    // later LaunchedEffect below covers switching servers mid-session, where a
    // push is both correct and stack-safe.
    val startDestination = remember {
        if (setupState.activeEnvId != null) RouteSetup else RouteHome
    }
    LaunchedEffect(setupState.activeEnvId) {
        if (setupState.activeEnvId != null) {
            navController.navigate(RouteSetup) { launchSingleTop = true }
        }
    }

    // Past the onboarding gate, so we only ask once a backend is connected.
    NotificationPermissionRequest()

    val backStackEntry by navController.currentBackStackEntryAsState()
    val pendingOpen by AppContainer.pendingOpenFromNotification.collectAsStateWithLifecycle()

    // Notification tap → open that session. Keyed on the observed value so it
    // fires for taps while the app is already running (onNewIntent), not only
    // on a cold start. ChatScreen loads the session detail on entry itself.
    LaunchedEffect(pendingOpen) {
        val id = AppContainer.consumePendingOpenFromNotification() ?: return@LaunchedEffect
        AppContainer.appStateHolder.openChatSession(id)
        navController.navigate(RouteChat) { launchSingleTop = true }
    }

    // Keep nav in sync when Home opens a chat / subagent details programmatically.
    LaunchedEffect(appState.activeTab, appState.selectedSessionId) {
        if (appState.activeTab == com.console.mobile.data.store.MobileTab.Chat && !appState.selectedSessionId.isNullOrBlank()) {
            val current = backStackEntry?.destination?.route ?: ""
            if (!current.contains("RouteChat", ignoreCase = true)) {
                navController.navigate(RouteChat) { launchSingleTop = true }
            }
        }
    }

    Scaffold(
        containerColor = NewTheme.Background,
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = startDestination,
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
                            com.console.mobile.data.store.MobileTab.Devices -> navController.navigate(RouteDevices) { launchSingleTop = true }
                            com.console.mobile.data.store.MobileTab.Subagents -> navController.navigate(RouteSubagents) { launchSingleTop = true }
                            else -> {}
                        }
                    },
                    onOpenSubagentDetails = { id -> navController.navigate(RouteSubagentDetails(id)) },
                    onAddProject = { navController.navigate(RouteAddProject) },
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
            composable<RouteDevices> {
                DevicesScreen(onBack = {
                    AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Chat)
                    navController.navigate(RouteChat) { launchSingleTop = true }
                })
            }
            composable<RouteSettings> {
                SettingsScreen(
                    onBackToHome = {
                        AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
                        navController.navigate(RouteHome) {
                            launchSingleTop = true
                            popUpTo(RouteHome)
                        }
                    },
                    onAddProject = { navController.navigate(RouteAddProject) },
                )
            }
            // Same screen, but it knows the first-run panel sent it here, so
            // back returns to that panel instead of the Settings landing list.
            composable<RouteSettingsFromSetup> {
                SettingsScreen(
                    onBackToHome = { navController.popBackStack() },
                    onAddProject = { navController.navigate(RouteAddProject) },
                    startSection = com.console.mobile.feature.settings.SettingsSection.Providers,
                    openedFromSetup = true,
                )
            }
            composable<RouteAddProject> {
                AddProjectScreen(
                    onBack = { navController.popBackStack() },
                    onAdded = { navController.popBackStack() },
                )
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
            composable<RouteSetup> {
                SetupScreen(
                    state = setupState,
                    onStep = { step ->
                        // Each step opens the screen it names. Routing every
                        // one of them to Settings was a dead end: "Add project"
                        // has a real AddProject screen two taps away.
                        //
                        // Deliberately not dismissed here - backing out of the
                        // destination returns to this panel, which is what
                        // pressing back should do.
                        when (step) {
                            SetupStep.AddProject -> navController.navigate(RouteAddProject)
                            // Lands directly on the Providers section rather
                            // than the Settings landing list: going to Settings
                            // and hunting for Providers was the "dead end".
                            SetupStep.Providers, SetupStep.Github ->
                                navController.navigate(RouteSettingsFromSetup)
                        }
                    },
                    // Both halves matter. `dismiss()` clears the state and
                    // persists it; without the pop the route stays on the back
                    // stack and SetupScreen keeps rendering — the button looked
                    // inert because closing the state was never going to change
                    // what the navigator was showing.
                    onSkip = {
                        AppContainer.setupRepository.dismiss()
                        navController.popBackStack()
                    },
                )
            }
        }
        ConfirmPromptHost()
    }
}

@Composable
fun PlaceholderScreen(label: String) {
    Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Text(label, color = NewTheme.TextSecondary)
    }
}
