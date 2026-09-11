package com.console.mobile.ui.navigation

import kotlinx.serialization.Serializable

/**
 * Typed destinations — mirrors apps/mobile/components/layout/main-content.tsx MobileTab
 * plus onboarding + subagent-details. Navigation Compose 2.8 + kotlinx.serialization.
 */
@Serializable object RouteOnboarding
@Serializable object RouteHome
@Serializable object RouteChat
@Serializable object RouteTerminal
@Serializable object RouteFiles
@Serializable object RouteChanges
@Serializable object RouteSubagents
@Serializable data class RouteSubagentDetails(val subagentId: String)
@Serializable object RouteSettings
