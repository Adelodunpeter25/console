package com.console.mobile.feature.chat

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.contextPercent
import com.console.mobile.ui.theme.ConsoleColors
import console.v1.ContextSnapshot

/**
 * Footer ring mirroring the desktop's context indicator. Hidden until a
 * snapshot is known; turns amber near the compaction threshold and red past it.
 */
@Composable
fun ContextRing(snapshot: ContextSnapshot?, modifier: Modifier = Modifier) {
    val pct = snapshot?.let { contextPercent(it.used_tokens, it.context_window, it.percent_used) } ?: return
    val threshold = snapshot.threshold_ratio.takeIf { it > 0.0 }?.times(100) ?: 90.0
    val tint = when {
        pct >= threshold -> ConsoleColors.Destructive
        pct >= threshold - 10 -> Color(0xFFFACC15)
        else -> ConsoleColors.TextSecondary
    }
    Row(modifier = modifier, verticalAlignment = Alignment.CenterVertically) {
        Canvas(modifier = Modifier.size(14.dp)) {
            val stroke = 2.dp.toPx()
            val inset = stroke / 2
            val arcSize = Size(size.width - stroke, size.height - stroke)
            drawArc(Color.White.copy(alpha = 0.12f), 0f, 360f, false, Offset(inset, inset), arcSize, style = Stroke(stroke))
            drawArc(tint, -90f, 360f * pct / 100f, false, Offset(inset, inset), arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
        }
        Text("$pct%", color = tint, fontSize = 11.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 5.dp))
    }
}
