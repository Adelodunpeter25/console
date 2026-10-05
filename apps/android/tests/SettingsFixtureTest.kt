package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ConsoleSettings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the settings migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/settings and are shared by Go, Rust, and Kotlin.
 */
class SettingsFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val settingsAdapter = moshi.adapter(ConsoleSettings::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/settings/$name").readText().trim()

    @Test
    fun decodesGoldenFullFixture() {
        val msg = settingsAdapter.fromJson(fixture("full.json"))!!
        assertEquals("openai/gpt-5", msg.model_roles?.vision)
        assertEquals("anthropic/claude-opus-4", msg.model_roles?.smol)
    }

    @Test
    fun decodesGoldenEmptyFixture() {
        val msg = settingsAdapter.fromJson(fixture("empty.json"))!!
        assertNull(msg.model_roles?.vision)
        assertNull(msg.model_roles?.smol)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = settingsAdapter.fromJson(
            "{\"modelRoles\":{\"vision\":\"x\",\"futureRole\":\"y\"},\"futureField\":1}",
        )!!
        assertEquals("x", msg.model_roles?.vision)
    }

    @Test
    fun serializesCamelCase() {
        val encoded = settingsAdapter.toJson(
            ConsoleSettings(
                model_roles = console.v1.ModelRoleMapping(vision = "openai/gpt-5"),
            ),
        )
        assertTrue("unexpected encoding: $encoded", encoded.contains("modelRoles"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("vision"))
    }
}
