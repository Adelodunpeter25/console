package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.DeviceActionRequest
import console.v1.DeviceDescriptor
import console.v1.DeviceDiagnostics
import console.v1.DeviceStreamMeta
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the device migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/device and are shared by Go, Rust, and Kotlin.
 */
class DeviceFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<DeviceDescriptor>>(
        Types.newParameterizedType(List::class.java, DeviceDescriptor::class.java),
    )
    private val diagnosticsAdapter = moshi.adapter(DeviceDiagnostics::class.java)
    private val metaAdapter = moshi.adapter(DeviceStreamMeta::class.java)
    private val actionAdapter = moshi.adapter(DeviceActionRequest::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/device/$name").readText().trim()

    @Test
    fun decodesGoldenListFixture() {
        val devs = listAdapter.fromJson(fixture("list.json"))!!
        assertEquals(2, devs.size)
        assertEquals("Pixel_7", devs[0].id)
        assertEquals("android", devs[0].platform)
        assertEquals("shutdown", devs[0].state)
        // Absent optionals read as null, never "".
        assertNull(devs[0].os_version)
        assertNull(devs[0].model)
        assertTrue(devs[0].is_available)
        assertEquals("26.3", devs[1].os_version)
    }

    @Test
    fun decodesGoldenDiagnosticsFixture() {
        // diskFreeBytes is uint64, so the wire carries a quoted string; Wire
        // surfaces it as a Long with the same value.
        val msg = diagnosticsAdapter.fromJson(fixture("diagnostics.json"))!!
        assertTrue(msg.xcode_installed)
        assertTrue(msg.simctl_available)
        assertTrue(msg.android_sdk_found)
        assertTrue(msg.adb_available)
        assertTrue(msg.emulator_available)
        assertEquals(111201239040L, msg.disk_free_bytes)
        assertTrue(msg.has_enough_disk_space)
        assertNull(msg.xcode_version)
        assertTrue(msg.errors.isEmpty())
    }

    @Test
    fun decodesGoldenStreamMetaFixture() {
        val msg = metaAdapter.fromJson(fixture("stream_meta.json"))!!
        assertEquals("meta", msg.type)
        assertEquals(1170, msg.width)
        assertEquals(2532, msg.height)
        assertEquals("iPad (A16)", msg.name)
        assertEquals("avc1.42E01E", msg.codec)
    }

    @Test
    fun decodesGoldenActionFixture() {
        val msg = actionAdapter.fromJson(fixture("action.json"))!!
        assertEquals("swipe", msg.action)
        assertEquals(100.5, msg.x!!, 1e-9)
        assertEquals(200.25, msg.y!!, 1e-9)
        assertEquals(250, msg.duration_ms)
        // Unused coordinates stay absent so the server's defaults apply.
        assertNull(msg.text)
        assertNull(msg.key)
        assertNull(msg.appearance)
    }

    @Test
    fun ignoresUnknownFields() {
        val devs = listAdapter.fromJson(
            """[{"id":"a","name":"b","platform":"ios","state":"booted","isAvailable":true,"futureField":"x"}]""",
        )!!
        assertEquals("a", devs[0].id)
        assertTrue(devs[0].is_available)
        assertFalse(devs[0].model != null)
    }

    @Test
    fun encodesActionAsCamelCase() {
        val raw = actionAdapter.toJson(DeviceActionRequest(action = "tap", x = 1.5, y = 2.0))
        assertTrue(raw, raw.contains("\"action\":\"tap\""))
        assertTrue(raw, raw.contains("\"x\":1.5"))
        // Absent optionals must not be written out.
        assertFalse(raw, raw.contains("endX"))
    }
}