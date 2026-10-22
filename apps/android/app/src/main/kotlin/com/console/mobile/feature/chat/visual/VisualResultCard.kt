package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.model.VisualArtifact
import com.console.mobile.ui.theme.NewTheme

/**
 * One native card for a `visualize` tool result. The spec is parsed once per
 * artifact; an unsupported version or malformed spec shows a compact fallback
 * and never throws into the transcript.
 */
@Composable
fun VisualResultCard(artifact: VisualArtifact, modifier: Modifier = Modifier) {
    val parsed = remember(artifact) { parseVisual(artifact) }
    when (parsed) {
        is VisualParse.Ok -> VisualCardBody(parsed.spec, modifier)
        VisualParse.Unsupported -> VisualFallback("This visual needs a newer version of Console.", modifier)
        VisualParse.Invalid -> VisualFallback("Visual unavailable", modifier)
    }
}

@Composable
private fun VisualFallback(message: String, modifier: Modifier) {
    VisualSurface(modifier) {
        Text(message, color = NewTheme.TextMuted, fontSize = 13.sp)
    }
}

@Composable
private fun VisualSurface(modifier: Modifier, content: @Composable () -> Unit) {
    Surface(
        modifier = modifier.fillMaxWidth().padding(vertical = 6.dp),
        color = NewTheme.Background,
        shape = RoundedCornerShape(12.dp),
    ) {
        Column(modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            content()
        }
    }
}

@Composable
private fun VisualCardBody(spec: VisualSpec, modifier: Modifier) {
    VisualSurface(modifier) {
        if (spec.title != null || spec.subtitle != null) {
            Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                spec.title?.let { Text(it, color = NewTheme.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold) }
                spec.subtitle?.let { Text(it, color = NewTheme.TextSecondary, fontSize = 12.sp) }
            }
        }
        // Consecutive metrics share a row; everything else stacks.
        var i = 0
        val blocks = spec.blocks
        while (i < blocks.size) {
            val b = blocks[i]
            if (b is VisualBlock.Metric) {
                val run = blocks.drop(i).takeWhile { it is VisualBlock.Metric }.filterIsInstance<VisualBlock.Metric>()
                VisualMetricRow(run)
                i += run.size
            } else {
                when (b) {
                    is VisualBlock.Text -> Text(b.text, color = NewTheme.TextSecondary, fontSize = 13.sp)
                    is VisualBlock.Table -> VisualTable(b)
                    is VisualBlock.Chart -> VisualChart(b)
                    is VisualBlock.Metric -> Unit
                }
                i++
            }
        }
    }
}
