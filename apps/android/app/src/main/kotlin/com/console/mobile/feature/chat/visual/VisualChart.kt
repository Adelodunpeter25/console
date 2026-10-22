package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.background
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.drawText
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import kotlin.math.max
import kotlin.math.min

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun VisualChart(chart: VisualBlock.Chart) {
    val description = remember(chart) { describeChart(chart) }
    val wide = LocalConfiguration.current.screenWidthDp >= 600
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        chart.title?.let { Text(it, color = NewTheme.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium) }
        ChartCanvas(chart, Modifier.fillMaxWidth().height(if (wide) 220.dp else 180.dp).semantics { contentDescription = description })
        if (chart.series.size > 1) {
            // Legend wraps below the plot, never over it.
            FlowRow(horizontalArrangement = Arrangement.spacedBy(14.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                chart.series.forEachIndexed { i, s ->
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        androidx.compose.foundation.layout.Box(
                            Modifier.size(8.dp).background(seriesColor(i), CircleShape),
                        )
                        Text(s.name, color = NewTheme.TextSecondary, fontSize = 11.sp)
                    }
                }
            }
        }
        val axes = listOfNotNull(chart.xLabel?.let { "x: $it" }, chart.yLabel?.let { "y: $it" })
        if (axes.isNotEmpty()) Text(axes.joinToString("   "), color = NewTheme.TextMuted, fontSize = 11.sp)
    }
}

private fun seriesColor(i: Int) = NewTheme.VisualSeries[i % NewTheme.VisualSeries.size]

@Composable
private fun ChartCanvas(chart: VisualBlock.Chart, modifier: Modifier) {
    val measurer = rememberTextMeasurer()
    val labelStyle = TextStyle(color = NewTheme.TextMuted, fontSize = 10.sp)
    val scatter = chart.kind == "scatter"
    val includeZero = chart.kind == "bar" || chart.kind == "area"
    val all = remember(chart) { chart.series.flatMap { it.points } }
    val yAxis = remember(chart) {
        valueAxis(all.minOfOrNull { it.y } ?: 0.0, all.maxOfOrNull { it.y } ?: 1.0, includeZero)
    }
    val xAxis = remember(chart) {
        if (scatter) valueAxis(all.minOfOrNull { it.x } ?: 0.0, all.maxOfOrNull { it.x } ?: 1.0, false) else null
    }
    val xMin = remember(chart) { all.minOfOrNull { it.x } ?: 0.0 }
    val xMax = remember(chart) { all.maxOfOrNull { it.x } ?: 1.0 }
    val gridColor = NewTheme.Divider
    val colors = chart.series.indices.map { seriesColor(it) }

    Canvas(modifier) {
        val yLabels = yAxis.ticks.map { measurer.measure(formatCompact(it), labelStyle) }
        val left = (yLabels.maxOfOrNull { it.size.width } ?: 0) + 8.dp.toPx()
        val bottom = 18.dp.toPx()
        val top = 6.dp.toPx()
        val right = 6.dp.toPx()
        val plot = Size(size.width - left - right, size.height - top - bottom)
        if (plot.width <= 0f || plot.height <= 0f) return@Canvas

        fun py(v: Double) = top + plot.height - ((v - yAxis.min) / (yAxis.max - yAxis.min)).toFloat() * plot.height
        val bars = chart.kind == "bar"
        val slots = if (bars) max(1, all.map { it.x }.distinct().size) else 0
        // Ordinal charts inset half a slot so bars/points do not touch the axes.
        fun px(x: Double): Float {
            if (scatter && xAxis != null) {
                return left + ((x - xAxis.min) / (xAxis.max - xAxis.min)).toFloat() * plot.width
            }
            if (xMax == xMin) return left + plot.width / 2
            val inset = if (bars) plot.width / slots / 2 else 0f
            return left + inset + ((x - xMin) / (xMax - xMin)).toFloat() * (plot.width - inset * 2)
        }

        yAxis.ticks.forEachIndexed { i, t ->
            val y = py(t)
            drawLine(gridColor, Offset(left, y), Offset(left + plot.width, y), strokeWidth = 1.dp.toPx())
            drawText(yLabels[i], topLeft = Offset(left - 6.dp.toPx() - yLabels[i].size.width, y - yLabels[i].size.height / 2f))
        }

        // X tick labels: numeric ticks for scatter, point labels (thinned) otherwise.
        val xTicks: List<Pair<Float, String>> = if (scatter && xAxis != null) {
            xAxis.ticks.map { px(it) to formatCompact(it) }
        } else {
            val ref = chart.series.firstOrNull { it.points.isNotEmpty() }?.points.orEmpty()
            val step = max(1, ref.size / 6)
            ref.filterIndexed { i, _ -> i % step == 0 }.map { px(it.x) to (it.label ?: formatCompact(it.x)) }
        }
        var lastEnd = -Float.MAX_VALUE
        xTicks.forEach { (x, text) ->
            val m = measurer.measure(text, labelStyle)
            val start = (x - m.size.width / 2f).coerceIn(0f, size.width - m.size.width)
            if (start > lastEnd + 4.dp.toPx()) {
                drawText(m, topLeft = Offset(start, top + plot.height + 3.dp.toPx()))
                lastEnd = start + m.size.width
            }
        }

        val zeroY = py(max(yAxis.min, min(0.0, yAxis.max)))
        // Pixel-density safety net; the server cap already bounds the data.
        val buckets = plot.width.toInt().coerceAtLeast(1)
        chart.series.forEachIndexed { si, s ->
            val color = colors[si]
            val pts = if (scatter || bars) s.points else minMaxBuckets(s.points, buckets)
            when (chart.kind) {
                "scatter" -> pts.forEach { drawCircle(color, 3.dp.toPx(), Offset(px(it.x), py(it.y))) }
                "bar" -> {
                    val per = plot.width / slots
                    val bw = min(24.dp.toPx(), per * 0.7f / chart.series.size)
                    pts.forEach {
                        val cx = px(it.x) + (si - (chart.series.size - 1) / 2f) * bw
                        val y = py(it.y)
                        drawRect(color, Offset(cx - bw / 2, min(y, zeroY)), Size(bw, kotlin.math.abs(zeroY - y).coerceAtLeast(1f)))
                    }
                }
                else -> {
                    if (pts.size == 1) {
                        drawCircle(color, 3.dp.toPx(), Offset(px(pts[0].x), py(pts[0].y)))
                    } else if (pts.size > 1) {
                        val line = Path().apply {
                            pts.forEachIndexed { i, p -> if (i == 0) moveTo(px(p.x), py(p.y)) else lineTo(px(p.x), py(p.y)) }
                        }
                        if (chart.kind == "area") {
                            val fill = Path().apply {
                                addPath(line)
                                lineTo(px(pts.last().x), zeroY)
                                lineTo(px(pts.first().x), zeroY)
                                close()
                            }
                            drawPath(fill, color.copy(alpha = 0.12f))
                        }
                        drawPath(line, color, style = Stroke(1.5.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round))
                    }
                }
            }
        }
    }
}
