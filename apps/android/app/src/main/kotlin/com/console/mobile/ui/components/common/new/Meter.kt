package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/**
 * Thin horizontal meter. [percent] is 0..100. A non-zero value always draws a
 * sliver, so "1%" isn't invisible.
 */
@Composable
fun MeterBar(percent: Double, fill: Color, modifier: Modifier = Modifier, height: Dp = 4.dp) {
    val fraction = (percent / 100.0).coerceIn(0.0, 1.0).toFloat().let { if (it > 0f) it.coerceAtLeast(0.015f) else 0f }
    Box(modifier = modifier.fillMaxWidth().height(height).clip(RoundedCornerShape(999.dp)).background(Color.White.copy(alpha = 0.1f))) {
        if (fraction > 0f) {
            Box(modifier = Modifier.fillMaxWidth(fraction).height(height).clip(RoundedCornerShape(999.dp)).background(fill))
        }
    }
}
