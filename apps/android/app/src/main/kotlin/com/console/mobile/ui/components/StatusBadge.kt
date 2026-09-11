package com.console.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.ui.theme.ConsoleColors

/** Status dot + label — mirrors session-list status pill. */
@Composable
fun StatusBadge(status: SessionStatus?, modifier: Modifier = Modifier) {
    val s = status ?: SessionStatus.Idle
    val (bg, fg, label) = when (s) {
        SessionStatus.Working -> Triple(ConsoleColors.StatusRunningBg, ConsoleColors.StatusRunning, "Working")
        SessionStatus.Done -> Triple(ConsoleColors.StatusReadyBg, ConsoleColors.StatusReady, "Ready")
        SessionStatus.NeedsAttention -> Triple(ConsoleColors.StatusAttentionBg, ConsoleColors.StatusAttention, "Attention")
        SessionStatus.Idle -> Triple(ConsoleColors.StatusIdleBg, ConsoleColors.StatusIdle, "Idle")
    }
    Box(modifier = modifier.clip(RoundedCornerShape(999.dp)).background(bg).padding(horizontal = 8.dp, vertical = 3.dp)) {
        Text(label, color = fg, fontSize = 10.sp)
    }
}
