package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import com.console.mobile.data.model.encodeKillFrame
import com.console.mobile.data.model.encodeResizeFrame
import console.v1.TerminalClientMessage
import console.v1.TerminalKill
import console.v1.TerminalResize
import console.v1.TerminalServerMessage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the terminal migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/terminal and are shared by Go, Rust, and Kotlin.
 * Raw PTY bytes and the tag framing are untouched and untested here.
 */
class TerminalFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val serverAdapter = moshi.adapter(TerminalServerMessage::class.java)
    private val clientAdapter = moshi.adapter(TerminalClientMessage::class.java)
    private val resizeAdapter = moshi.adapter(TerminalResize::class.java)
    private val killAdapter = moshi.adapter(TerminalKill::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/terminal/$name").readText().trim()

    @Test
    fun decodesGoldenServerFrames() {
        val spawned = serverAdapter.fromJson(fixture("spawned.json"))!!
        val s = (spawned.event as TerminalServerMessage.Event.Spawned).value
        assertEquals("t1", s.id)
        assertEquals(123, s.pid)
        assertEquals(80, s.cols)
        val output = serverAdapter.fromJson(fixture("output.json"))!!
        assertEquals("hi\n", (output.event as TerminalServerMessage.Event.Output).value.data_)
        val exit = serverAdapter.fromJson(fixture("exit.json"))!!
        assertEquals(0, (exit.event as TerminalServerMessage.Event.Exit).value.code)
        val error = serverAdapter.fromJson(fixture("error.json"))!!
        assertEquals("boom", (error.event as TerminalServerMessage.Event.Error).value.message)
    }

    @Test
    fun decodesGoldenClientFrames() {
        val input = clientAdapter.fromJson(fixture("input.json"))!!
        assertEquals("ls\n", (input.event as TerminalClientMessage.Event.Input).value.data_)
        val resize = clientAdapter.fromJson(fixture("resize.json"))!!
        val r = (resize.event as TerminalClientMessage.Event.Resize).value
        assertEquals(100, r.cols)
        assertEquals(30, r.rows)
        val kill = clientAdapter.fromJson(fixture("kill.json"))!!
        assertTrue(kill.event is TerminalClientMessage.Event.Kill)
    }

    @Test
    fun ignoresUnknownFields() {
        // Dropped spawned label must be ignored, not fatal.
        val msg = serverAdapter.fromJson(
            "{\"spawned\":{\"id\":\"t\",\"pid\":1,\"cwd\":\"/\",\"shell\":\"sh\",\"cols\":80,\"rows\":24,\"label\":\"x\"}}",
        )!!
        assertEquals("t", (msg.event as TerminalServerMessage.Event.Spawned).value.id)
    }

    @Test
    fun serializesControlFrames() {
        val resize = resizeAdapter.toJson(TerminalResize(cols = 100, rows = 30))
        assertTrue("unexpected encoding: $resize", resize.contains("\"cols\":100"))
        val kill = killAdapter.toJson(TerminalKill())
        assertEquals("{}", kill)
    }

    // Regression: the app used to send the bare TerminalResize, which the server
    // rejects as "Unknown terminal frame type", leaving the PTY at 80x24.
    @Test
    fun controlFramesMatchTheSharedClientFixtures() {
        assertEquals(fixture("resize.json"), encodeResizeFrame(100, 30))
        assertEquals(fixture("kill.json"), encodeKillFrame())
    }
}
