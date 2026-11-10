package com.console.mobile.feature.chat

import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme

/** Plan mode reads at a glance: faint diagonal stripes over a card. Apply after the background. */
fun Modifier.planModeStripes(): Modifier = drawBehind {
    val step = 11.dp.toPx()
    val stroke = 1.2.dp.toPx()
    var x = -size.height
    while (x < size.width) {
        drawLine(
            color = NewTheme.Accent.copy(alpha = 0.14f),
            start = Offset(x, size.height),
            end = Offset(x + size.height, 0f),
            strokeWidth = stroke,
        )
        x += step
    }
}
