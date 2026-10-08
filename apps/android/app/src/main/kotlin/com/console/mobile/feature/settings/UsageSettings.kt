package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Circle
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatUsageValue
import com.console.mobile.core.util.limitTone
import com.console.mobile.core.util.usageResetLabel
import com.console.mobile.core.util.usedPercent
import com.console.mobile.ui.components.ProviderIcon
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.color
import console.v1.UsageLimit
import console.v1.UsageReport
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors

/**
 * Port of screens/settings/usage-settings.tsx + usage-provider-card + usage-limit-row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsageSettings(onBack: () -> Unit) {
    val usageState by AppContainer.usageStateHolder.state.collectAsStateWithLifecycle()
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    var refreshing by remember { mutableStateOf(false) }

    LaunchedEffect(Unit) {
        AppContainer.authRepository.loadStatus()
        AppContainer.usageRepository.loadAllUsage()
    }

    val cards = listOf(
        Triple("antigravity", "Google Antigravity", authState.status?.get("antigravity")),
        Triple("codex", "OpenAI Codex", authState.status?.get("codex")),
        Triple("claude", "Anthropic Claude", authState.status?.get("claude")),
    )
    val isLoading = usageState.loading && usageState.reports.isEmpty()

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Usage",
            onBack = onBack,
            actions = {
                IconButton(onClick = { AppContainer.usageRepository.loadAllUsage(force = true) }, modifier = Modifier.size(36.dp)) {
                    Icon(TablerIcons.Outline.Refresh, contentDescription = "Refresh", tint = ConsoleColors.TextPrimary, modifier = Modifier.size(16.dp))
                }
            },
        )
        if (isLoading) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                    Text("Loading quota…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                }
            }
        } else {
            PullToRefreshBox(
                isRefreshing = refreshing,
                onRefresh = {
                    refreshing = true
                    AppContainer.usageRepository.loadAllUsage(force = true)
                    refreshing = false
                },
                modifier = Modifier.fillMaxSize(),
            ) {
                Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                    Text("Remaining quota for your signed-in providers. Pull to refresh.", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 12.dp).padding(top = 8.dp))
                    cards.forEach { (key, displayName, auth) ->
                        UsageProviderCard(provider = key, displayName = displayName, report = usageState.reports[key], loggedIn = auth?.logged_in == true, email = auth?.email)
                    }
                }
            }
        }
    }
}

@Composable
private fun UsageProviderCard(provider: String, displayName: String, report: UsageReport?, loggedIn: Boolean, email: String?) {
    val mostPressured = report?.limits?.firstOrNull()
    val pressure = mostPressured?.let { usedPercent(it) }
    // Heading is the provider; the card holds its limits (or why there are none).
    SettingsCategory(displayName) {
        Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            ProviderIcon(provider = provider, sizeDp = 24)
            Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
                Text(if (loggedIn) (email ?: "Connected") else "Not connected", color = if (loggedIn) NewTheme.TextPrimary else NewTheme.TextMuted, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            if (mostPressured != null && pressure != null) {
                Text("${pressure.toInt()}%", color = limitTone(mostPressured, pressure).color(), fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
            }
        }
        when {
            !loggedIn -> {
                SettingsDivider()
                SettingsNote("Sign in under Providers to see quota.")
            }
            report == null -> {
                SettingsDivider()
                Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Warning, modifier = Modifier.size(18.dp))
                    Text("Quota unavailable — token expired, project missing, or billing disabled. Re-login under Providers.", color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(start = 12.dp))
                }
            }
            report.limits.isEmpty() -> {
                SettingsDivider()
                SettingsNote("No limits reported.")
            }
            else -> report.limits.forEach { limit ->
                SettingsDivider()
                UsageLimitRow(limit)
            }
        }
    }
}

@Composable
private fun UsageLimitRow(limit: UsageLimit) {
    val percent = usedPercent(limit)
    val tone = limitTone(limit, percent).color()
    val reset = usageResetLabel(limit)
    val scope = listOfNotNull(
        limit.scope?.tier?.takeIf { it.isNotBlank() },
        limit.scope?.model_id?.takeIf { it.isNotBlank() },
    ).joinToString(" · ")
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(limit.label, color = NewTheme.TextPrimary, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(end = 8.dp))
            Text(formatUsageValue(limit, percent), color = tone, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
        }
        val sub = listOfNotNull(scope.ifEmpty { null }, reset).joinToString(" · ")
        if (sub.isNotEmpty()) {
            Text(sub, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
        }
        Box(modifier = Modifier.fillMaxWidth().padding(top = 8.dp).height(5.dp).clip(RoundedCornerShape(999.dp)).background(Color.White.copy(alpha = 0.1f))) {
            val fraction = (percent / 100.0).toFloat().let { if (it > 0f) it.coerceIn(0.015f, 1f) else 0f }
            if (fraction > 0f) Box(modifier = Modifier.fillMaxWidth(fraction).height(5.dp).clip(RoundedCornerShape(999.dp)).background(tone))
        }
    }
}

