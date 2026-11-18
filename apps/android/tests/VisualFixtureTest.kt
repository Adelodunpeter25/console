package com.console.mobile

import com.console.mobile.data.model.VisualArtifact
import com.console.mobile.data.model.toUi
import com.console.mobile.feature.chat.visual.VisualBlock
import com.console.mobile.feature.chat.visual.VisualParse
import com.console.mobile.feature.chat.visual.parseVisual
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentMessage as WireMessage
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Workspace image visuals (read_file), HtmlPart wire decode, and legacy spec fallback. */
class VisualFixtureTest {
    private val adapter = Moshi.Builder().add(WireJsonAdapterFactory()).build().adapter(WireMessage::class.java)

    private fun testdata(path: String) = File("../../../proto/testdata/$path").readText().trim()
    private fun artifact(name: String) =
        VisualArtifact(1, Json.parseToJsonElement(testdata("visual/$name")))

    @Test
    fun parsesImageBlock() {
        val withAlt = parseVisual(
            VisualArtifact(1, Json.parseToJsonElement("""{"version":1,"blocks":[{"type":"image","path":"/workspace/assets/chart.png","alt":"Quarterly revenue chart"}]}""")),
        ) as VisualParse.Ok
        val image = withAlt.spec.blocks.single() as VisualBlock.Image
        assertEquals("/workspace/assets/chart.png", image.path)
        assertEquals("Quarterly revenue chart", image.alt)

        val bare = parseVisual(
            VisualArtifact(1, Json.parseToJsonElement("""{"version":1,"blocks":[{"type":"image","path":"/a.png"}]}""")),
        ) as VisualParse.Ok
        val bareImage = bare.spec.blocks.single() as VisualBlock.Image
        assertEquals(null, bareImage.alt)
    }

    @Test
    fun legacySpecsAndGarbageFallBack() {
        assertTrue(parseVisual(VisualArtifact(2, Json.parseToJsonElement("{}"))) is VisualParse.Unsupported)
        assertTrue(parseVisual(artifact("dashboard.json")) is VisualParse.Invalid)
        assertTrue(parseVisual(VisualArtifact(1, Json.parseToJsonElement("""{"version":1,"blocks":[{"type":"nope"}]}"""))) is VisualParse.Invalid)
    }

    @Test
    fun decodesHtmlPartFromToolResult() {
        val json = """{"toolResult":{"results":[{"toolCallId":"c1","toolName":"render","content":"e30=","html":{"path":"/tmp/page.html","title":"Sidebar","height":640,"snapshot":"iVBORw0KGgo="}}]}}"""
        val msg = adapter.fromJson(json)!!.toUi()!!
        check(msg is com.console.mobile.data.model.ToolResultMessage)
        val html = msg.results.single().html
        assertNotNull(html)
        assertEquals("/tmp/page.html", html!!.path)
        assertEquals("Sidebar", html.title)
        assertEquals(640, html.height)
        assertEquals("iVBORw0KGgo=", html.snapshotBase64)
    }
}
