package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.unit.dp
import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.sin
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle

/**
 * Material 3 Expressive-style morphing shape indicator: smoothly transitions
 * between a 4-lobe clover, a soft circle, and a 6-lobe scalloped flower while
 * rotating.
 */
@Composable
fun ExpressiveLoadingIndicator(
    color: Color = NewTheme.Accent,
    modifier: Modifier = Modifier.size(36.dp),
) {
    val transition = rememberInfiniteTransition(label = "expressiveLoader")
    val rotation by transition.animateFloat(
        initialValue = 0f,
        targetValue = 360f,
        animationSpec = infiniteRepeatable(tween(2400, easing = LinearEasing), RepeatMode.Restart),
        label = "loaderRotation",
    )
    val phase by transition.animateFloat(
        initialValue = 0f,
        targetValue = 3f,
        animationSpec = infiniteRepeatable(tween(2100, easing = FastOutSlowInEasing), RepeatMode.Restart),
        label = "loaderMorph",
    )
    val path = remember { Path() }
    Canvas(modifier = modifier) {
        val cx = size.width / 2f
        val cy = size.height / 2f
        val maxR = size.minDimension / 2f
        val p = phase % 3f
        val w4: Float
        val w6: Float
        when {
            p < 1f -> {
                w4 = 1f - p
                w6 = 0f
            }
            p < 2f -> {
                w4 = 0f
                w6 = p - 1f
            }
            else -> {
                val t = p - 2f
                w4 = t
                w6 = 1f - t
            }
        }
        val steps = 72
        path.reset()
        for (i in 0..steps) {
            val theta = (i.toFloat() / steps) * (2f * PI.toFloat())
            val r = maxR * (0.82f + 0.16f * w4 * cos(4f * theta) + 0.12f * w6 * cos(6f * theta))
            val x = cx + r * cos(theta)
            val y = cy + r * sin(theta)
            if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
        }
        path.close()
        rotate(rotation) {
            drawPath(path = path, color = color)
        }
    }
}

/** Centered expressive loading indicator with an optional caption, for a screen that is still loading. */
@Composable
fun LoadingState(caption: String? = null, modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            ExpressiveLoadingIndicator(color = NewTheme.Accent, modifier = Modifier.size(36.dp))
            if (caption != null) {
                Text(
                    caption,
                    color = NewTheme.TextSecondary,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(top = 12.dp),
                )
            }
        }
    }
}

/** A tinted notice (errors by default) that sits in the page, outside any [Section]. */
@Composable
fun Banner(text: String, tint: Color = NewTheme.Danger, modifier: Modifier = Modifier) {
    val shape = RoundedCornerShape(NewTheme.FieldRadius)
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(shape)
            .background(tint.copy(alpha = 0.12f))
            .border(1.dp, tint.copy(alpha = 0.24f), shape)
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
        Text(text, color = tint, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(start = 10.dp))
    }
}
