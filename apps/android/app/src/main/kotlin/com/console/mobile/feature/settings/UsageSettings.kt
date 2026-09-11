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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AlertTriangle
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material.icons.filled.Refresh
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
import androidx.compose.runtime.rememberCoroutineScope
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
import com.console.mobile.AppContainer
import com.console.mobile.core.util.colorForLimit
import com.console.mobile.core.util.formatWindowLabel
import com.console.mobile.core.util.getBarPercent
import com.console.mobile.core.util.getUsedPercent
import com.console.mobile.core.util.statusForLimit
import com.console.mobile.data.model.UsageLimit
import com.console.mobile.data.model.UsageReport
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
    )
    val isLoading = usageState.loading && usageState.reports.isEmpty()

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Usage",
            onBack = onBack,
            actions = {
                IconButton(onClick = { AppContainer.usageRepository.loadAllUsage(force = true) }, modifier = Modifier.size(36.dp)) {
                    Icon(Icons.Filled.Refresh, contentDescription = "Refresh", tint = ConsoleColors.TextPrimary, modifier = Modifier.size(16.dp))
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
                    Text("Remaining quota for your signed-in providers. Pull to refresh.", color = ConsoleColors.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(horizontal = 4.dp).padding(top = 8.dp, bottom = 16.dp))
                    cards.forEach { (key, displayName, auth) ->
                        UsageProviderCard(displayName = displayName, report = usageState.reports[key], loggedIn = auth?.loggedIn == true, email = auth?.email)
                    }
                    scope.toString()
                }
            }
        }
    }
}

@Composable
private fun UsageProviderCard(displayName: String, report: UsageReport?, loggedIn: Boolean, email: String?) {
    val isExhausted = report?.limits?.any { statusForLimit(it) == "exhausted" } == true
    val mostPressured = report?.limits?.firstOrNull()
    val cardShape = RoundedCornerShape(16.dp)
    Column(
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).clip(cardShape)
            .background(ConsoleColors.Card)
            .border(1.dp, ConsoleColors.Border, cardShape)
            .padding(16.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(bottom = 8.dp)) {
            if (loggedIn) Icon(Icons.Filled.Check, contentDescription = null, tint = if (isExhausted) Color(0xFFF87171) else Color(0xFF34D399), modifier = Modifier.size(14.dp))
            else Icon(Icons.Filled.RadioButtonUnchecked, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
            Column(modifier = Modifier.weight(1f).padding(start = 8.dp)) {
                Text(displayName, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                var sub = if (loggedIn) (email ?: "Connected") else "Not connected"
                Text(sub, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
            }
            val used = mostPressured?.amount?.used
            if (mostPressured != null && used != null) {
                Text("${Math.round(used)}%", color = parseUsageColor(colorForLimit(mostPressured)), fontSize = 12.sp, fontWeight = FontWeight.Bold)
            }
        }
        if (!loggedIn) {
            Box(modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp), contentAlignment = Alignment.Center) {
                Text("Sign in via Account to see quota.", color = ConsoleColors.TextSecondary, fontSize = 12.sp)
            }
        } else if (report == null) {
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(vertical = 12.dp)) {
                Icon(Icons.Filled.AlertTriangle, contentDescription = null, tint = Color(0xFFFBBF24), modifier = Modifier.size(14.dp))
                Text("Quota unavailable — token expired, project missing, or billing disabled. Re-login in Account.", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 8.dp))
            }
        } else if (report.limits.isEmpty()) {
            Text("No limits reported.", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(vertical = 12.dp))
        } else {
            report.limits.forEach { UsageLimitRow(it) }
        }
    }
}

@Composable
private fun UsageLimitRow(limit: UsageLimit) {
    val usedPct = getUsedPercent(limit)
    val remainingPct = limit.amount.remainingFraction?.let { Math.round(it * 1000) / 10.0 }
    val barPct = getBarPercent(limit).toFloat() / 100f
    val color = parseUsageColor(colorForLimit(limit))
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f).padding(end = 8.dp)) {
                Text(limit.label, color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                var sub = formatWindowLabel(limit)
                if (!limit.scope.tier.isNullOrBlank()) sub = "${limit.scope.tier} · $sub"
                if (!limit.scope.modelId.isNullOrBlank()) sub = "$sub · ${limit.scope.modelId}"
                Text(sub, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
            }
            Column(horizontalAlignment = Alignment.End) {
                Text(
                    when {
                        usedPct != null -> "$usedPct% used"
                        remainingPct != null -> "$remainingPct% left"
                        !limit.status.isNullOrBlank() -> limit.status
                        else -> "—"
                    },
                    color = color, fontSize = 12.sp, fontWeight = FontWeight.Bold,
                )
                if (limit.amount.remaining != null && limit.amount.remainingFraction != null) {
                    Text("${String.format("%.1f", limit.amount.remaining)}% remaining", color = ConsoleColors.TextSecondary, fontSize = 11.sp)
                }
            }
        }
        Box(modifier = Modifier.fillMaxWidth().padding(top = 6.dp).height(6.dp).clip(RoundedCornerShape(999.dp)).background(Color.White.copy(alpha = 0.1f))) {
            Box(modifier = Modifier.fillMaxWidth(barPct).height(6.dp).clip(RoundedCornerShape(999.dp)).background(color))
        }
    }
}

private fun parseUsageColor(hex: String): Color {
    return try {
        Color(android.graphics.Color.parseColor(hex))
    } catch (_: Exception) {
        ConsoleColors.TextMuted
    }
}
