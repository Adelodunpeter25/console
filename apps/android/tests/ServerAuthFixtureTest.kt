package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.PairDeviceResponse
import console.v1.PairingGrantResponse
import console.v1.ServerDevicesResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the server-pairing payloads. The fixtures live in
 * proto/testdata/auth and are shared by Go, Rust, and Kotlin — the payloads
 * are the shared protobuf types, so all three must read them identically.
 */
class ServerAuthFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val pairAdapter = moshi.adapter(PairDeviceResponse::class.java)
    private val grantAdapter = moshi.adapter(PairingGrantResponse::class.java)
    private val devicesAdapter = moshi.adapter(ServerDevicesResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/auth/$name").readText().trim()

    @Test
    fun decodesGoldenPairDeviceFixture() {
        val data = pairAdapter.fromJson(fixture("pair_device.json"))!!
        assertEquals("ct1_device_token", data.token)
        // Timestamps arrive as protojson strings, not numbers.
        assertEquals(1762592000000L, data.expires_at)
        val device = data.device
        assertNotNull("device must be reported", device)
        assertEquals("device-1", device!!.id)
        assertEquals("My desktop", device.name)
        assertEquals("desktop", device.platform)
        // The server says which machine issued the token, so the phone can
        // confirm it paired with the one the user meant.
        val server = data.server
        assertNotNull("server must be reported", server)
        assertEquals("server-1", server!!.id)
        assertEquals("Studio Mac", server.name)
        assertEquals("1.0.1", server.version)
    }

    @Test
    fun decodesGoldenPairingGrantFixture() {
        val grant = grantAdapter.fromJson(fixture("pairing_grant.json"))!!
        assertEquals("cp1_grant", grant.grant)
        assertEquals(1760000030000L, grant.expires_at)
        val server = grant.server
        assertNotNull("server must be reported", server)
        assertEquals("server-1", server!!.id)
        // The QR code the other device scans travels as PNG bytes, and it
        // has to arrive intact: a phone that mangles it scans nothing.
        val qr = grant.qr_png.toByteArray()
        assertEquals("the QR must arrive as PNG bytes", 68, qr.size)
        assertTrue("the QR must be a PNG", qr.copyOfRange(0, 8).contentEquals(byteArrayOf(0x89.toByte(), 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A)))
    }

    @Test
    fun decodesGoldenServerDevicesFixture() {
        val list = devicesAdapter.fromJson(fixture("server_devices.json"))!!
        assertEquals(2, list.devices.size)
        val desktop = list.devices[0]
        assertEquals("device-1", desktop.id)
        assertEquals(1760000000000L, desktop.created_at)
        // A device that has never been seen carries no timestamp at all.
        assertNull(desktop.last_seen_at)
        assertNull(desktop.revoked_at)
        val phone = list.devices[1]
        assertEquals(1760000099000L, phone.last_seen_at)
        assertEquals(1760000100000L, phone.revoked_at)
    }
}
