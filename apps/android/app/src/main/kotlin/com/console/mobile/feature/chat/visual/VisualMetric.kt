package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun VisualMetricRow(metrics: List<VisualBlock.Metric>) {
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        metrics.forEach { VisualMetric(it, Modifier.widthIn(min = 96.dp)) }
    }
}

@Composable
fun VisualMetric(m: VisualBlock.Metric, modifier: Modifier = Modifier) {
    Column(modifier, verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(m.label, color = NewTheme.TextSecondary, fontSize = 12.sp)
        Text(m.value, color = NewTheme.TextPrimary, fontSize = 22.sp, fontWeight = FontWeight.SemiBold)
        m.delta?.let { delta ->
            // Arrow glyph plus color, so the trend is never color-only.
            val (arrow, tint) = when (m.trend) {
                "up" -> "▲ " to NewTheme.Success
                "down" -> "▼ " to NewTheme.Danger
                "flat" -> "▬ " to NewTheme.TextSecondary
                else -> "" to NewTheme.TextSecondary
            }
            Text(arrow + delta, color = tint, fontSize = 12.sp, fontWeight = FontWeight.Medium)
        }
        m.caption?.let { Text(it, color = NewTheme.TextMuted, fontSize = 11.sp) }
    }
}
