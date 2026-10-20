package com.console.mobile

import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.DeviceInfo
import com.console.mobile.data.model.DeviceListData
import com.console.mobile.data.model.PairedDevice
import com.console.mobile.data.model.PairingData
import com.console.mobile.data.model.PairingGrantData
import com.console.mobile.data.model.PairingGrantRequest
import com.console.mobile.data.model.PairingRequest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Wire shapes for server device authentication.
 *
 * These are hand-written DTOs against routes that are deliberately
 * breaking ("no backwards compatibility"), so a field rename must fail here
 * rather than at runtime on a real server.
 */
class ServerAuthModelsTest {
    @Test
    fun pairingRequestUsesServerFieldNames() {
        val json = ConsoleJson.encodeToString(
            PairingRequest.serializer(),
            PairingRequest(grant = "cp1_abc", deviceName = "Pixel"),
        )
        assertEquals(
            """{"grant":"cp1_abc","deviceName":"Pixel","platform":"android"}""",
            json,
        )
    }

    @Test
    fun pairingResponseDecodesDeviceTokenAndExpiry() {
        val data = ConsoleJson.decodeFromString(
            PairingData.serializer(),
            """{"device":{"id":"d1","name":"Pixel","platform":"android"},"token":"ct1_x","expiresAt":1762592000000}""",
        )
        assertEquals(PairedDevice("d1", "Pixel", "android"), data.device)
        assertEquals("ct1_x", data.token)
        assertEquals(1762592000000L, data.expiresAt)
    }

    @Test
    fun pairingGrantRequestTargetsTheNewDevice() {
        val json = ConsoleJson.encodeToString(
            PairingGrantRequest.serializer(),
            PairingGrantRequest(deviceName = "Pixel"),
        )
        assertEquals("""{"deviceName":"Pixel","platform":"android"}""", json)
        val data = ConsoleJson.decodeFromString(
            PairingGrantData.serializer(),
            """{"grant":"cp1_qr","expiresAt":123}""",
        )
        assertEquals("cp1_qr", data.grant)
    }

    @Test
    fun deviceListDecodesOptionalTimestamps() {
        val data = ConsoleJson.decodeFromString(
            DeviceListData.serializer(),
            """{"devices":[{"id":"d1","name":"Desktop","platform":"desktop","createdAt":1}]}""",
        )
        assertEquals(1, data.devices.size)
        val device: DeviceInfo = data.devices.first()
        assertEquals(1L, device.createdAt)
        assertNull(device.lastSeenAt)
    }

    @Test
    fun emptyDeviceListIsEmptyNotMissing() {
        val data = ConsoleJson.decodeFromString(DeviceListData.serializer(), """{}""")
        assertEquals(0, data.devices.size)
    }
}
