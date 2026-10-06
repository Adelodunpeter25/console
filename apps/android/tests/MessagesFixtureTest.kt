package com.console.mobile

import com.console.mobile.data.model.toUi
import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.AgentMessage as WireMessage
import console.v1.ToolCall as WireToolCall
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the messages migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/message and are shared by Go, Rust, and Kotlin.
 */
class MessagesFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val messageAdapter = moshi.adapter(WireMessage::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/message/$name").readText().trim()

    @Test
    fun decodesGoldenUserFixture() {
        val ui = messageAdapter.fromJson(fixture("user.json"))!!.toUi()!!
        check(ui is com.console.mobile.data.model.UserMessage)
        assertEquals("Hello", ui.content)
        assertEquals(1, ui.attachments?.size)
        assertEquals(listOf("/a.ts"), ui.contextFiles)
    }

    @Test
    fun decodesGoldenAssistantFixture() {
        val ui = messageAdapter.fromJson(fixture("assistant.json"))!!.toUi()!!
        check(ui is com.console.mobile.data.model.AssistantMessage)
        assertEquals(3, ui.content.size)
        val call = (ui.content[2] as com.console.mobile.data.model.ToolCallPart).call
        assertEquals("c1", call.id)
        assertEquals("/a", call.arguments?.jsonObject?.get("path")?.jsonPrimitive?.content)
    }

    @Test
    fun decodesGoldenToolResultFixture() {
        val ui = messageAdapter.fromJson(fixture("toolresult.json"))!!.toUi()!!
        check(ui is com.console.mobile.data.model.ToolResultMessage)
        assertEquals("c1", ui.results[0].toolCallId)
        assertEquals(true, ui.results[0].content?.jsonObject?.get("ok")?.jsonPrimitive?.boolean)
    }

    @Test
    fun ignoresUnknownFields() {
        val ui = messageAdapter.fromJson(
            "{\"user\":{\"content\":\"x\",\"annotations\":[{\"id\":\"a\"}]},\"futureField\":1}",
        )!!.toUi()!!
        check(ui is com.console.mobile.data.model.UserMessage)
        assertEquals("x", ui.content)
    }

    @Test
    fun serializesBytesAsBase64() {
        // Tool arguments cross as raw JSON bytes (base64 on the wire).
        val encoded = messageAdapter.toJson(
            WireMessage(
                message = WireMessage.Message.Assistant(
                    console.v1.AgentAssistantMessage(
                        id = "m1",
                        content = listOf(
                            console.v1.AssistantContentPart(
                                part = console.v1.AssistantContentPart.Part.ToolCall(
                                    WireToolCall(id = "c1", name = "read", arguments = okio.Buffer().writeUtf8("{\"path\":\"/a\"}").readByteString()),
                                ),
                            ),
                        ),
                        stop_reason = "stop",
                    ),
                ),
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("eyJwYXRoIjoiL2EifQ=="))
    }
}
