package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
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
import com.console.mobile.core.util.UsageTone
import com.console.mobile.core.util.contextTone
import com.console.mobile.core.util.formatContextUsage
import com.console.mobile.core.util.formatUsageValue
import com.console.mobile.core.util.limitTone
import com.console.mobile.core.util.usageResetLabel
import com.console.mobile.core.util.usedPercent
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.color
import console.v1.UsageLimit
import com.console.mobile.ui.components.common.new.MeterBar
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.Note

/**
 * The desktop usage popover as a sheet: context occupancy first, then the
 * provider's quota limits for the model this chat is using.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun UsageSheet(sessionId: String, onDismiss: () -> Unit) {
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val usage by AppContainer.usageStateHolder.state.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()

    val view = sessionViews[sessionId]
    val provider = view?.sessionProvider?.takeIf { it.isNotBlank() }
    // Fresh numbers on every open (the repository still de-dupes within 30s).
    LaunchedEffect(provider) { provider?.let { AppContainer.usageRepository.loadUsage(it) } }
    LaunchedEffect(sessionId) { AppContainer.chatRepository.loadContext(sessionId) }

    val snapshot = chatSessions[sessionId]?.context
    val report = provider?.let { usage.reports[it] }
    val loading = provider != null && usage.loadingByProvider[provider] == true

    BaseSheet(onDismiss = onDismiss, title = "Usage") {
        // ---- Context
        Row(modifier = Modifier.padding(horizontal = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("Context", color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium, modifier = Modifier.weight(1f))
            if (snapshot != null) {
                Text(
                    formatContextUsage(snapshot.used_tokens.toLong(), snapshot.context_window.toLong()),
                    color = NewTheme.TextSecondary, fontSize = 13.sp,
                )
            }
        }
        if (snapshot != null) {
            val percent = snapshot.percent_used.coerceIn(0.0, 100.0)
            MeterBar(percent, contextTone(percent, snapshot.threshold_ratio).color(), modifier = Modifier.padding(horizontal = 4.dp, vertical = 12.dp))
        } else {
            Text("Context usage unavailable.", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp))
        }

        // ---- Provider limits
        Text(
            "Usage limits",
            color = NewTheme.Accent,
            fontSize = 15.sp,
            fontWeight = FontWeight.Medium,
            modifier = Modifier.padding(horizontal = 4.dp).padding(top = 22.dp, bottom = 8.dp),
        )
        when {
            loading && report == null -> Note("Loading usage data…")
            report == null -> Note("No quota limits reported for this provider.")
            report.limits.isEmpty() -> Note("No active rate limit windows.")
            else -> report.limits.forEachIndexed { i, limit ->
                if (i > 0) Box(modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp).height(1.dp).background(NewTheme.Divider))
                LimitRow(limit)
            }
        }
    }
}

@Composable
private fun LimitRow(limit: UsageLimit) {
    val percent = usedPercent(limit)
    val reset = usageResetLabel(limit)
    // Desktop doesn't show scope, but two "Weekly" rows for different models are
    // indistinguishable without it.
    val scope = listOfNotNull(
        limit.scope?.tier?.takeIf { it.isNotBlank() },
        limit.scope?.model_id?.takeIf { it.isNotBlank() },
    ).joinToString(" · ")
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp, vertical = 12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(limit.label, color = NewTheme.TextPrimary, fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(end = 8.dp))
            if (reset != null) {
                Text(reset, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, modifier = Modifier.padding(end = 8.dp))
            }
            Text(formatUsageValue(limit, percent), color = limitTone(limit, percent).color(), fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
        }
        if (scope.isNotEmpty()) {
            Text(scope, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
        }
        MeterBar(percent, limitTone(limit, percent).color(), modifier = Modifier.padding(top = 7.dp))
    }
}
