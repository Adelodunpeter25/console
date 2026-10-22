package com.console.mobile.feature.chat.visual

import java.util.Locale
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.floor
import kotlin.math.log10
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow

/** Pure scale/tick math for the chart painter; no Compose types so it is unit-testable. */
data class Axis(val min: Double, val max: Double, val ticks: List<Double>)

fun niceStep(range: Double, targetTicks: Int): Double {
    val raw = range / max(1, targetTicks)
    val mag = 10.0.pow(floor(log10(raw)))
    val norm = raw / mag
    val nice = when {
        norm <= 1.0 -> 1.0
        norm <= 2.0 -> 2.0
        norm <= 5.0 -> 5.0
        else -> 10.0
    }
    return nice * mag
}

/**
 * Value axis for [lo]..[hi]. Bars and areas must include zero; lines and
 * scatter use the data range with padding. A flat series is expanded so the
 * line sits mid-plot instead of dividing by zero.
 */
fun valueAxis(lo: Double, hi: Double, includeZero: Boolean, targetTicks: Int = 5): Axis {
    var a = lo
    var b = hi
    if (includeZero) {
        a = min(a, 0.0)
        b = max(b, 0.0)
    }
    if (a == b) {
        val pad = if (a == 0.0) 1.0 else abs(a) * 0.5
        a -= pad
        b += pad
    } else if (!includeZero) {
        val pad = (b - a) * 0.05
        a -= pad
        b += pad
    }
    val step = niceStep(b - a, targetTicks)
    val niceMin = floor(a / step) * step
    val niceMax = ceil(b / step) * step
    val ticks = ArrayList<Double>()
    var t = niceMin
    var guard = 0
    while (t <= niceMax + step * 1e-9 && guard++ < 40) {
        ticks += if (abs(t) < step * 1e-9) 0.0 else t
        t += step
    }
    return Axis(niceMin, niceMax, ticks)
}

/** Compact tick text: 1.2k, 3.4M; integers stay bare. */
fun formatCompact(v: Double, locale: Locale = Locale.getDefault()): String {
    val a = abs(v)
    return when {
        a >= 1e9 -> trim(v / 1e9, locale, 1) + "B"
        a >= 1e6 -> trim(v / 1e6, locale, 1) + "M"
        a >= 1e3 -> trim(v / 1e3, locale, 1) + "k"
        else -> trim(v, locale, 2)
    }
}

private fun trim(v: Double, locale: Locale, decimals: Int): String {
    val s = String.format(locale, "%.${decimals}f", v)
    val sep = s.indexOfFirst { it == '.' || it == ',' }
    if (sep < 0) return s
    return s.trimEnd('0').trimEnd('.', ',')
}

/**
 * Rendering safety net: at most one min and one max per horizontal bucket, so
 * a dense line never draws more segments than pixels. Does not change data
 * for series already under the cap.
 */
fun minMaxBuckets(points: List<VisualPoint>, buckets: Int): List<VisualPoint> {
    if (buckets < 1 || points.size <= buckets * 2) return points
    val out = ArrayList<VisualPoint>(buckets * 2)
    val per = points.size.toDouble() / buckets
    for (i in 0 until buckets) {
        val from = (i * per).toInt()
        val to = min(points.size, ((i + 1) * per).toInt()).coerceAtLeast(from + 1)
        var lo = points[from]
        var hi = points[from]
        for (j in from until to) {
            if (points[j].y < lo.y) lo = points[j]
            if (points[j].y > hi.y) hi = points[j]
        }
        if (lo.x <= hi.x) { out += lo; if (hi !== lo) out += hi } else { out += hi; out += lo }
    }
    return out
}

/** Generated TalkBack summary: kind, title, series/point counts, range, last value. */
fun describeChart(chart: VisualBlock.Chart): String {
    val all = chart.series.flatMap { it.points }
    val kind = chart.kind.replaceFirstChar { it.uppercase() }
    val title = chart.title?.let { ", $it" }.orEmpty()
    val seriesCount = chart.series.size
    val perSeries = chart.series.maxOfOrNull { it.points.size } ?: 0
    val range = if (all.isEmpty()) "" else
        " Range ${formatCompact(all.minOf { it.y })} to ${formatCompact(all.maxOf { it.y })}." +
            chart.series.lastOrNull()?.points?.lastOrNull()?.let { " Last ${formatCompact(it.y)}." }.orEmpty()
    return "$kind chart$title. ${plural(seriesCount, "series", "series")}, ${plural(perSeries, "point", "points")}.$range"
}

private fun plural(n: Int, one: String, many: String) = "$n ${if (n == 1) one else many}"

/** Heuristic from the plan: numeric-looking columns right-align. */
fun looksNumeric(cell: String): Boolean {
    val t = cell.trim().removePrefix("+").removePrefix("-").removePrefix("$").removePrefix("€").removePrefix("£")
    if (t.isEmpty()) return false
    val core = t.trimEnd('%', 'k', 'K', 'M', 'B')
    return core.isNotEmpty() && core.all { it.isDigit() || it == '.' || it == ',' } && core.any { it.isDigit() }
}
