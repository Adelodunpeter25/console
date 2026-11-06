package com.console.mobile.ui.navigation

import kotlinx.serialization.Serializable

/**
 * Typed destinations — mirrors apps/mobile/components/layout/main-content.tsx MobileTab
 * plus onboarding. Navigation Compose 2.8 + kotlinx.serialization.
 */
@Serializable object RouteOnboarding
@Serializable object RouteSetup
@Serializable object RouteHome
@Serializable object RouteChat
@Serializable object RouteTerminal
@Serializable object RouteFiles
@Serializable object RouteDevices
@Serializable object RoutePorts
@Serializable object RouteChanges
@Serializable data class RouteBrowser(val url: String)
@Serializable object RouteSettings
@Serializable object RouteSettingsFromSetup
// Settings, opened straight on Providers — where the GitHub token row lives.
// The project pickers' "Connect GitHub…" hands off here.
@Serializable object RouteSettingsProviders
@Serializable object RouteAddProject
