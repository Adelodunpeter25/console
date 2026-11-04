package com.console.mobile

import com.console.mobile.data.repo.ScriptsRepository
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ProjectScript
import console.v1.ProjectScriptsResult
import console.v1.ScriptRun
import console.v1.ScriptRunEvent
import console.v1.StopScriptRunResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the project-scripts domain (the sixth shared
 * protobuf schema, proto/console/v1/scripts.proto). Fixtures live in
 * proto/testdata/scripts and are shared by Go, Rust, and Kotlin.
 */
class ScriptsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val resultAdapter = moshi.adapter(ProjectScriptsResult::class.java)
    private val scriptAdapter = moshi.adapter(ProjectScript::class.java)
    private val runAdapter = moshi.adapter(ScriptRun::class.java)
    private val eventAdapter = moshi.adapter(ScriptRunEvent::class.java)
    private val stopAdapter = moshi.adapter(StopScriptRunResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/scripts/$name").readText().trim()

    @Test
    fun decodesGoldenScriptsFixture() {
        val result = resultAdapter.fromJson(fixture("scripts_result.json"))!!
        assertEquals("p1", result.project_id)
        assertEquals("console.toml", result.source)
        assertEquals(1, result.scripts.size)
        assertEquals("dev", result.scripts[0].id)
        assertEquals("Dev server", result.scripts[0].label)
        assertEquals("bun dev", result.scripts[0].command)
        // Unset shortcuts encode as "" on the wire, never null.
        assertEquals("", result.scripts[0].shortcut)
        assertTrue(result.scripts[0].persistent)
    }

    @Test
    fun decodesGoldenScriptFixture() {
        val script = scriptAdapter.fromJson(fixture("script.json"))!!
        assertEquals("dev", script.id)
        assertEquals("Dev server", script.label)
        assertEquals("bun dev", script.command)
    }

    @Test
    fun decodesGoldenRunFixture() {
        val run = runAdapter.fromJson(fixture("run.json"))!!
        assertEquals("r1", run.run_id)
        assertEquals("p1", run.project_id)
        assertEquals("dev", run.script_id)
        assertEquals("running", run.status)
        assertEquals("hi\n", run.stdout)
        assertNull(run.exit_code)
        assertNull(run.ended_at)
    }

    @Test
    fun decodesGoldenStatusEventFixture() {
        val event = eventAdapter.fromJson(fixture("event_status.json"))!!
        val status = event.event as? ScriptRunEvent.Event.Status
        assertEquals("running", status?.value?.status)
    }

    @Test
    fun decodesGoldenOutputEventFixture() {
        val event = eventAdapter.fromJson(fixture("event_output.json"))!!
        val output = event.event as? ScriptRunEvent.Event.Output
        assertEquals("stdout", output?.value?.stream)
        assertEquals("hi\n", output?.value?.text)
    }

    @Test
    fun decodesGoldenExitEventFixture() {
        val event = eventAdapter.fromJson(fixture("event_exit.json"))!!
        val exit = event.event as? ScriptRunEvent.Event.Exit
        assertEquals("failed", exit?.value?.status)
        assertEquals(3, exit?.value?.exit_code)
    }

    @Test
    fun decodesGoldenStopFixture() {
        val stop = stopAdapter.fromJson(fixture("stop.json"))!!
        assertTrue(stop.stopped)
    }

    @Test
    fun findsDevServerPortInRunOutput() {
        // The shapes Vite/Next/Bun print, and the ones the row's Open action
        // offers. Localhost first match wins.
        assertEquals(
            5173,
            ScriptsRepository.portInOutput("  ➜  Local:   http://localhost:5173/"),
        )
        assertEquals(3000, ScriptsRepository.portInOutput("ready on http://127.0.0.1:3000"))
        assertEquals(8080, ScriptsRepository.portInOutput("listening on 0.0.0.0:8080"))
        assertNull(ScriptsRepository.portInOutput("compiled successfully in 400ms"))
    }
}
