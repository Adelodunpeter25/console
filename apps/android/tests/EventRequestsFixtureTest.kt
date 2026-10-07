package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AskQuestionRequest
import console.v1.BrowserActionRequest
import console.v1.ErrorPayload
import console.v1.PermissionRequest
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the interactive request frames migration (Phase
 * 4, fourth slice). Fixtures live in proto/testdata/event and are shared by
 * Go, Rust, and Kotlin. Frames stay hand-shaped; request payloads are
 * schema'd (args as JSON bytes).
 */
class EventRequestsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()

    private fun fixture(name: String): String =
        File("../../../proto/testdata/event/$name").readText().trim()

    private fun payload(name: String, key: String): String =
        kotlinx.serialization.json.Json.parseToJsonElement(fixture(name)).jsonObject.getValue(key).toString()

    @Test
    fun decodesGoldenPermissionFrame() {
        val req = moshi.adapter(PermissionRequest::class.java).fromJson(payload("permission_request.json", "request"))!!
        assertEquals("r1", req.request_id)
        assertEquals("c1", req.tool_call_id)
        assertEquals("sh", req.tool_name)
        assertEquals("{\"path\":\"a\"}", req.args.utf8())
        assertEquals("write", req.tier)
        assertEquals("needs review", req.reason)
    }

    @Test
    fun decodesGoldenAskFrame() {
        val req = moshi.adapter(AskQuestionRequest::class.java).fromJson(payload("ask_question.json", "request"))!!
        assertEquals("r1", req.request_id)
        assertEquals("q?", req.question)
        assertEquals(listOf("a", "b"), req.options)
        assertTrue(req.is_multi_select)
        // Explicit false stays present — never confused with absent.
        assertEquals(false, req.skippable)
        assertEquals("b1", req.batch_id)
    }

    @Test
    fun decodesGoldenBrowserFrame() {
        val req = moshi.adapter(BrowserActionRequest::class.java).fromJson(payload("browser_action.json", "request"))!!
        assertEquals("r1", req.request_id)
        assertEquals("click", req.action)
        assertEquals("#b", req.selector)
        assertEquals(5000, req.timeout_ms)
        assertNull(req.ref)
        assertEquals(false, req.submit)
    }

    @Test
    fun decodesGoldenErrorFrame() {
        val raw = kotlinx.serialization.json.Json.parseToJsonElement(fixture("error.json")).jsonObject.getValue("error").toString()
        assertEquals("boom", moshi.adapter(ErrorPayload::class.java).fromJson(raw)!!.message)
    }

    @Test
    fun ignoresUnknownFields() {
        val req = moshi.adapter(PermissionRequest::class.java)
            .fromJson("{\"requestId\":\"r\",\"toolCallId\":\"c\",\"toolName\":\"n\",\"tier\":\"read\",\"futureField\":1}")!!
        assertEquals("r", req.request_id)
    }
}
