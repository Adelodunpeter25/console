package com.console.mobile

import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.stream.attachRequestJson
import com.console.mobile.data.stream.runBodyToWtStart
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

/**
 * The REST → WebTransport run-payload translation (plan Phase 3).
 *
 * Mobile reuses the server's protojson on both transports, so this is a field
 * rename rather than a second DTO mapping — which is exactly why it is worth
 * pinning. A drift here shows up as a run that starts with an empty prompt
 * rather than an error, so the assertions are about what the server will read.
 */
class WtPayloadTest {
    @Test
    fun promptBecomesText() {
        val wt = ConsoleJson.parseToJsonElement(
            runBodyToWtStart("""{"prompt":"fix the bug"}""", "s-1"),
        ).jsonObject

        assertEquals("s-1", wt["sessionId"]!!.jsonPrimitive.content)
        assertEquals("fix the bug", wt["start"]!!.jsonObject["text"]!!.jsonPrimitive.content)
        assertFalse(
            "the server reads `text`, not the REST `prompt`",
            wt["start"]!!.jsonObject.containsKey("prompt"),
        )
    }

    @Test
    fun optionalFieldsPassThroughUnchanged() {
        val wt = ConsoleJson.parseToJsonElement(
            runBodyToWtStart(
                """{"prompt":"hi","modelId":"m1","provider":"codex","approvalMode":"acceptEdits","thinkingLevel":"high","contextFiles":["src/main.rs"]}""",
                "s-2",
            ),
        ).jsonObject["start"]!!.jsonObject

        assertEquals("m1", wt["modelId"]!!.jsonPrimitive.content)
        assertEquals("codex", wt["provider"]!!.jsonPrimitive.content)
        assertEquals("acceptEdits", wt["approvalMode"]!!.jsonPrimitive.content)
        assertEquals("high", wt["thinkingLevel"]!!.jsonPrimitive.content)
        assertEquals(
            listOf("src/main.rs"),
            (wt["contextFiles"] as JsonArray).map { it.jsonPrimitive.content },
        )
    }

    @Test
    fun unsetOptionalFieldsAreOmitted() {
        // protojson omits unset values; sending empty strings would override the
        // server's own defaults instead of deferring to them.
        val wt = ConsoleJson.parseToJsonElement(
            runBodyToWtStart("""{"prompt":"hi"}""", "s-3"),
        ).jsonObject["start"]!!.jsonObject

        assertEquals(setOf("text"), wt.keys)
    }

    @Test
    fun attachCarriesTheReplayCursor() {
        val withSince = ConsoleJson.parseToJsonElement(attachRequestJson("s-4", 42)).jsonObject
        assertEquals("s-4", withSince["sessionId"]!!.jsonPrimitive.content)
        assertEquals(42L, withSince["attach"]!!.jsonObject["since"]!!.jsonPrimitive.content.toLong())

        // `since=null` must still send an attach request — omitting the cursor
        // means "wait for live frames", not "replay everything".
        val withoutSince = ConsoleJson.parseToJsonElement(attachRequestJson("s-5", null)).jsonObject
        assertTrue(withoutSince.containsKey("attach"))
        assertFalse(withoutSince["attach"]!!.jsonObject.containsKey("since"))
    }
}