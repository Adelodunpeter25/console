package com.console.mobile.ui.navigation

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.ui.unit.IntOffset
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.console.mobile.AppContainer
import com.console.mobile.core.notification.NotificationPermissionRequest
import com.console.mobile.feature.browser.BrowserScreen
import com.console.mobile.feature.browser.BrowserUriHandler
import com.console.mobile.feature.browser.handOffToSystem
import com.console.mobile.feature.browser.rememberBrowserState
import com.console.mobile.feature.changes.ChangesScreen
import com.console.mobile.feature.chat.ChatScreen
import com.console.mobile.feature.computer.ComputerScreen
import com.console.mobile.feature.devices.DevicesScreen
import com.console.mobile.feature.chat.html.HtmlPreviewScreen
import com.console.mobile.feature.files.FilesScreen
import com.console.mobile.feature.home.HomeScreen
import com.console.mobile.feature.onboarding.OnboardingScreen
import com.console.mobile.feature.onboarding.SetupScreen
import com.console.mobile.feature.onboarding.SetupStep
import com.console.mobile.feature.ports.PortsScreen
import com.console.mobile.feature.settings.AddProjectScreen
import com.console.mobile.feature.settings.SettingsScreen
import com.console.mobile.feature.settings.UsageSettings
import com.console.mobile.feature.terminal.TerminalScreen
import com.console.mobile.ui.components.common.new.ConfirmPromptHost
import com.console.mobile.ui.components.common.new.FloatingNavBar
import com.console.mobile.ui.components.common.new.FloatingNavTab
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.X

// Fractions of the screen width: the incoming page travels further than the
// outgoing one, which gives the push its depth.
private const val PAGE_ENTER_SHIFT = 0.28f
private const val PAGE_EXIT_SHIFT = 0.12f
private val PageSlideSpring = spring<IntOffset>(
    dampingRatio = Spring.DampingRatioNoBouncy,
    stiffness = Spring.StiffnessMediumLow,
)
private val PageFadeSpring = spring<Float>(
    dampingRatio = Spring.DampingRatioNoBouncy,
    stiffness = Spring.StiffnessMedium,
)

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

    // Keep activeTab in sync with the visible destination (including system Back
    // and notification taps) so foreground notification suppression is accurate.
    LaunchedEffect(backStackEntry) {
        val route = backStackEntry?.destination?.route ?: return@LaunchedEffect
        val tab = when {
            route.contains("RouteChat", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Chat
            route.contains("RouteHome", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Home
            route.contains("RouteFiles", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Files
            route.contains("RouteChanges", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Changes
            route.contains("RouteTerminal", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Terminal
            route.contains("RouteDevices", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Devices
            route.contains("RouteComputer", ignoreCase = true) -> com.console.mobile.data.store.MobileTab.Computer
            else -> null
        }
        if (tab != null && appState.activeTab != tab) {
            AppContainer.appStateHolder.setActiveTab(tab)
        }
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

    // Links in markdown open in the app's own browser rather than handing off
    // to Chrome (the ones worth tapping point at a forwarded port or tunnel on
    // the server this app already talks to), and file paths open in the Files
    // tab. Overriding LocalUriHandler is the whole hook: Compose's text links
    // read it, so every markdown renderer below comes through here without
    // knowing. Provided above the NavHost so every screen gets it.
    val systemUriHandler = LocalUriHandler.current
    val linkHandler = remember(systemUriHandler) {
        BrowserUriHandler(
            system = systemUriHandler,
            openFile = { path ->
                AppContainer.appStateHolder.openFile(path)
                navController.navigate(RouteFiles) { launchSingleTop = true }
            },
        ) { url -> navController.navigate(RouteBrowser(url)) { launchSingleTop = true } }
    }
    CompositionLocalProvider(LocalUriHandler provides linkHandler) {
    val currentRoute = backStackEntry?.destination?.route.orEmpty()
    val activeNavTab = when {
        currentRoute.contains("RouteUsage", ignoreCase = true) -> FloatingNavTab.Usage
        currentRoute.contains("RouteSettings", ignoreCase = true) &&
            !currentRoute.contains("RouteSettingsFromSetup", ignoreCase = true) &&
            !currentRoute.contains("RouteSettingsProviders", ignoreCase = true) -> FloatingNavTab.Settings
        currentRoute.contains("RouteHome", ignoreCase = true) -> FloatingNavTab.Chats
        else -> null
    }
    Scaffold(
        containerColor = NewTheme.Background,
    ) { padding ->
        Box(modifier = Modifier.fillMaxSize().padding(padding)) {
        NavHost(
            navController = navController,
            startDestination = startDestination,
            modifier = Modifier.fillMaxSize(),
            // Forward pushes the new screen in from the right while the old
            // one drifts left; back is the same motion reversed.
            enterTransition = {
                slideInHorizontally(PageSlideSpring) { (it * PAGE_ENTER_SHIFT).toInt() } +
                    fadeIn(PageFadeSpring)
            },
            exitTransition = {
                slideOutHorizontally(PageSlideSpring) { -(it * PAGE_EXIT_SHIFT).toInt() } +
                    fadeOut(PageFadeSpring)
            },
            popEnterTransition = {
                slideInHorizontally(PageSlideSpring) { -(it * PAGE_EXIT_SHIFT).toInt() } +
                    fadeIn(PageFadeSpring)
            },
            popExitTransition = {
                slideOutHorizontally(PageSlideSpring) { (it * PAGE_ENTER_SHIFT).toInt() } +
                    fadeOut(PageFadeSpring)
            },
        ) {
            composable<RouteHome> {
                HomeScreen(
                    onOpenSettings = { navController.navigate(RouteSettings) },
                    onOpenChat = { navController.navigate(RouteChat) { launchSingleTop = true } },
                )
            }
            composable<RouteChat> {
                run {
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
                                com.console.mobile.data.store.MobileTab.Computer -> navController.navigate(RouteComputer) { launchSingleTop = true }
                                else -> {}
                            }
                        },
                        onAddProject = { navController.navigate(RouteAddProject) },
                        onConnectGithub = { navController.navigate(RouteSettingsProviders) },
                        onOpenPorts = { navController.navigate(RoutePorts) { launchSingleTop = true } },
                        onOpenUrl = { url -> navController.navigate(RouteBrowser(url)) { launchSingleTop = true } },
                        onOpenHtmlPreview = { artifact ->
                            navController.navigate(RouteHtmlPreview(artifact.path, artifact.title)) { launchSingleTop = true }
                        },
                    )
                }
            }
            // A link target, opened as a push from wherever the link lived, so
            // back returns to that screen. In-page history is the screen's own
            // business (BrowserScreen consumes back while it can go back).
            composable<RouteBrowser> { entry ->
                val url = entry.toRoute<RouteBrowser>().url
                BrowserScreen(
                    state = rememberBrowserState(url),
                    toolbarTrailing = {
                        IconButton(onClick = { navController.popBackStack() }) {
                            Icon(
                                TablerIcons.Outline.X,
                                contentDescription = "Close browser",
                                tint = NewTheme.TextPrimary,
                                modifier = Modifier.size(20.dp),
                            )
                        }
                    },
                    // Schemes the WebView will not render still belong to a
                    // system app.
                    onExternalUrl = { handOffToSystem(systemUriHandler, it) },
                )
            }
            composable<RouteHtmlPreview> { entry ->
                val route = entry.toRoute<RouteHtmlPreview>()
                HtmlPreviewScreen(
                    path = route.path,
                    title = route.title,
                    onBack = { navController.popBackStack() },
                    onOpenEditor = { path ->
                        AppContainer.appStateHolder.openFile(path, forceEditor = true)
                        navController.navigate(RouteFiles) { launchSingleTop = true }
                    },
                    onOpenExternalUrl = { handOffToSystem(systemUriHandler, it) },
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
            composable<RouteComputer> {
                ComputerScreen(onBack = {
                    AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Chat)
                    navController.navigate(RouteChat) { launchSingleTop = true }
                })
            }
            // Ports is reached from chat's overflow menu rather than the tab
            // bar, so back is a plain pop to wherever it was pushed from.
            composable<RoutePorts> {
                PortsScreen(
                    onBack = { navController.popBackStack() },
                    onOpenPort = { url -> navController.navigate(RouteBrowser(url)) { launchSingleTop = true } },
                )
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
            composable<RouteUsage> {
                UsageSettings(onBack = null)
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
            // The pickers' "Connect GitHub…" handoff: straight to the GitHub
            // token row, and back returns to wherever the picker lived.
            composable<RouteSettingsProviders> {
                SettingsScreen(
                    onBackToHome = { navController.popBackStack() },
                    onAddProject = { navController.navigate(RouteAddProject) },
                    startSection = com.console.mobile.feature.settings.SettingsSection.Providers,
                )
            }
            composable<RouteAddProject> {
                AddProjectScreen(
                    onBack = { navController.popBackStack() },
                    onAdded = { navController.popBackStack() },
                )
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
        var lastNavTab by remember { androidx.compose.runtime.mutableStateOf(FloatingNavTab.Chats) }
        if (activeNavTab != null) lastNavTab = activeNavTab
        AnimatedVisibility(
            visible = activeNavTab != null,
            enter = slideInVertically(PageSlideSpring) { it } + fadeIn(PageFadeSpring),
            exit = slideOutVertically(PageSlideSpring) { it } + fadeOut(PageFadeSpring),
            modifier = Modifier.align(Alignment.BottomCenter),
        ) {
            FloatingNavBar(
                activeTab = activeNavTab ?: lastNavTab,
                onSelectTab = { tab ->
                    when (tab) {
                        FloatingNavTab.Chats -> {
                            AppContainer.appStateHolder.setActiveTab(com.console.mobile.data.store.MobileTab.Home)
                            navController.navigate(RouteHome) {
                                launchSingleTop = true
                                popUpTo(RouteHome)
                            }
                        }
                        FloatingNavTab.PullRequests -> {
                            // No-op for now.
                        }
                        FloatingNavTab.Usage -> {
                            navController.navigate(RouteUsage) { launchSingleTop = true }
                        }
                        FloatingNavTab.Settings -> {
                            navController.navigate(RouteSettings) { launchSingleTop = true }
                        }
                    }
                },
                onNewChat = {
                    val proj = AppContainer.projectStateHolder.state.value.projects.firstOrNull()
                    val id = AppContainer.sessionRepository.startNewChat(proj?.id, proj?.path)
                    AppContainer.appStateHolder.openChatSession(id)
                    navController.navigate(RouteChat) { launchSingleTop = true }
                },
            )
        }
        }
        ConfirmPromptHost()
    }
    }
}

@Composable
fun PlaceholderScreen(label: String) {
    Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Text(label, color = NewTheme.TextSecondary)
    }
}
