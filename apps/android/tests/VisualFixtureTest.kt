package com.console.mobile

import com.console.mobile.data.model.VisualArtifact
import com.console.mobile.data.model.toUi
import com.console.mobile.feature.chat.visual.VisualBlock
import com.console.mobile.feature.chat.visual.VisualParse
import com.console.mobile.feature.chat.visual.VisualPoint
import com.console.mobile.feature.chat.visual.describeChart
import com.console.mobile.feature.chat.visual.formatCompact
import com.console.mobile.feature.chat.visual.looksNumeric
import com.console.mobile.feature.chat.visual.minMaxBuckets
import com.console.mobile.feature.chat.visual.parseVisual
import com.console.mobile.feature.chat.visual.valueAxis
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentMessage as WireMessage
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Shared fixtures (proto/testdata/visual, message/toolresult_visual.json) plus geometry edge cases. */
class VisualFixtureTest {
    private val adapter = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(WireMessage::class.java)

    private fun testdata(path: String) = File("../../../proto/testdata/$path").readText().trim()
    private fun artifact(name: String) =
        VisualArtifact(1, Json.parseToJsonElement(testdata("visual/$name")))

    @Test
    fun decodesToolResultVisualFixture() {
        val msg = adapter.fromJson(testdata("message/toolresult_visual.json"))!!.toUi()!!
        check(msg is com.console.mobile.data.model.ToolResultMessage)
        val visual = msg.results.single().visual!!
        assertEquals(1, visual.version)
        val parsed = parseVisual(visual) as VisualParse.Ok
        assertEquals("Weekly signups", parsed.spec.title)
        val chart = parsed.spec.blocks.single() as VisualBlock.Chart
        assertEquals("line", chart.kind)
        assertEquals(3, chart.series.single().points.size)
    }

    @Test
    fun parsesDashboardBlocks() {
        val spec = (parseVisual(artifact("dashboard.json")) as VisualParse.Ok).spec
        assertEquals(
            listOf("metric", "metric", "chart", "table", "text"),
            spec.blocks.map {
                when (it) {
                    is VisualBlock.Metric -> "metric"
                    is VisualBlock.Chart -> "chart"
                    is VisualBlock.Table -> "table"
                    is VisualBlock.Text -> "text"
                }
            },
        )
    }

    @Test
    fun newerVersionAndGarbageFallBack() {
        assertTrue(parseVisual(VisualArtifact(2, Json.parseToJsonElement("{}"))) is VisualParse.Unsupported)
        assertTrue(parseVisual(VisualArtifact(1, Json.parseToJsonElement("""{"version":1,"blocks":[{"type":"nope"}]}"""))) is VisualParse.Invalid)
    }

    @Test
    fun flatSeriesGetsARange() {
        val a = valueAxis(5.0, 5.0, includeZero = false)
        assertTrue(a.min < 5.0 && a.max > 5.0)
    }

    @Test
    fun barsIncludeZeroAndNegativesWork() {
        val bars = valueAxis(40.0, 90.0, includeZero = true)
        assertEquals(0.0, bars.min, 0.0)
        val neg = valueAxis(-30.0, 10.0, includeZero = true)
        assertTrue(neg.min <= -30.0 && neg.ticks.contains(0.0))
    }

    @Test
    fun singlePointAxisIsFinite() {
        val a = valueAxis(0.0, 0.0, includeZero = true)
        assertTrue(a.ticks.size >= 2 && a.min < a.max)
    }

    @Test
    fun twoThousandPointsAreBucketed() {
        val pts = (0 until 2000).map { VisualPoint(it.toDouble(), (it % 17).toDouble()) }
        val out = minMaxBuckets(pts, 300)
        assertTrue(out.size <= 600)
        assertEquals(0.0, out.minOf { it.y }, 0.0)
        assertEquals(16.0, out.maxOf { it.y }, 0.0)
        assertEquals(pts, minMaxBuckets(pts, 1500)) // under the cap: untouched
    }

    @Test
    fun formatsAndDescribes() {
        assertEquals("1.2k", formatCompact(1234.0, java.util.Locale.US))
        assertEquals("3", formatCompact(3.0, java.util.Locale.US))
        assertTrue(looksNumeric("$42.1k") && looksNumeric("49.8%") && !looksNumeric("Organic"))
        val chart = (parseVisual(artifact("line.json")) as VisualParse.Ok).spec.blocks.single() as VisualBlock.Chart
        val d = describeChart(chart)
        assertTrue(d.startsWith("Line chart, Signups by week. 1 series, 3 points."))
        assertTrue(d.contains("Last 164"))
    }
}
