package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.ui.theme.NewTheme

/** Colour a chat status is shown in. Working takes the brand accent. */
fun SessionStatus.tint(): Color = when (this) {
    SessionStatus.Working -> NewTheme.Accent
    SessionStatus.Done -> NewTheme.Success
    SessionStatus.NeedsAttention -> NewTheme.Danger
    SessionStatus.Idle -> NewTheme.TextSecondary
}

/** The label shown for a chat status. */
fun SessionStatus.label(): String = when (this) {
    SessionStatus.Working -> "Working"
    SessionStatus.Done -> "Ready"
    SessionStatus.NeedsAttention -> "Attention"
    SessionStatus.Idle -> "Idle"
}

/** A small tinted pill with a chat's status. A null status reads as Idle. */
@Composable
fun StatusPill(status: SessionStatus?, modifier: Modifier = Modifier) {
    val s = status ?: SessionStatus.Idle
    val tint = s.tint()
    Text(
        s.label(), color = tint, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, maxLines = 1,
        modifier = modifier.clip(RoundedCornerShape(999.dp)).background(tint.copy(alpha = 0.14f)).padding(horizontal = 10.dp, vertical = 4.dp),
    )
}
