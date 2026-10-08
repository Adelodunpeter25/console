package com.console.mobile.feature.chat

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
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
import com.console.mobile.core.util.UsageTone
import com.console.mobile.core.util.ringTone
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.color
import console.v1.ContextSnapshot

/**
 * Footer ring mirroring the desktop's context indicator. Tapping opens the usage
 * sheet. With no snapshot yet it still draws an empty track so the sheet (which
 * also holds the provider limits) stays reachable; it turns amber near the
 * compaction threshold and red past it.
 */
@Composable
fun ContextRing(snapshot: ContextSnapshot?, onClick: () -> Unit, modifier: Modifier = Modifier) {
    val pct = snapshot?.let { contextPercent(it.used_tokens, it.context_window, it.percent_used) }
    // Blue like desktop's gauge, going amber at 80% and red at 95%.
    val tone = if (pct == null) UsageTone.Normal else ringTone(pct.toDouble())
    val tint = tone.color()
    Row(
        // Padding is inside the clickable so the touch target is larger than the 14dp glyph.
        modifier = modifier.clip(RoundedCornerShape(8.dp)).clickable(onClickLabel = "Show usage", onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Canvas(modifier = Modifier.size(14.dp)) {
            val stroke = 2.dp.toPx()
            val inset = stroke / 2
            val arcSize = Size(size.width - stroke, size.height - stroke)
            drawArc(Color.White.copy(alpha = 0.12f), 0f, 360f, false, Offset(inset, inset), arcSize, style = Stroke(stroke))
            if (pct != null) {
                drawArc(tint, -90f, 360f * pct / 100f, false, Offset(inset, inset), arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
            }
        }
        if (pct != null) {
            // The arc carries the blue; the number only takes on a colour once it matters.
            Text("$pct%", color = if (tone == UsageTone.Normal) NewTheme.TextSecondary else tint, fontSize = 11.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 5.dp))
        }
    }
}
