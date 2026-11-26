package com.console.mobile.ui.components.common.new

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
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

/** A small pill in [tint] on a faint wash of the same colour. */
@Composable
fun TintPill(
    text: String,
    tint: Color,
    modifier: Modifier = Modifier,
    pulsingDot: Boolean = false,
) {
    val shape = RoundedCornerShape(999.dp)
    Row(
        modifier = modifier
            .clip(shape)
            .background(tint.copy(alpha = 0.14f))
            .border(1.dp, tint.copy(alpha = 0.25f), shape)
            .padding(horizontal = 10.dp, vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        if (pulsingDot) {
            val transition = rememberInfiniteTransition(label = "statusPulse")
            val alpha by transition.animateFloat(
                initialValue = 0.35f,
                targetValue = 1f,
                animationSpec = infiniteRepeatable(tween(750), RepeatMode.Reverse),
                label = "statusDotAlpha",
            )
            Box(
                modifier = Modifier
                    .size(6.dp)
                    .graphicsLayer { this.alpha = alpha }
                    .clip(CircleShape)
                    .background(tint),
            )
        }
        Text(text, color = tint, fontSize = 11.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
    }
}

/** A small tinted pill with a chat's status. A null status reads as Idle. */
@Composable
fun StatusPill(status: SessionStatus?, modifier: Modifier = Modifier) {
    val s = status ?: SessionStatus.Idle
    TintPill(s.label(), s.tint(), modifier, pulsingDot = s == SessionStatus.Working)
}
