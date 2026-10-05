package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.GetApiVersionResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture test for the shared protobuf schema
 * (docs/plan/shared-protobuf-schema.md §8). The fixture lives in
 * proto/testdata and is shared by Go, Rust, and Kotlin.
 */
class VersionFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val adapter = moshi.adapter(GetApiVersionResponse::class.java)

    @Test
    fun decodesGoldenFixture() {
        val json = File("../../../proto/testdata/common/version.json").readText()
        val msg = adapter.fromJson(json)!!
        assertEquals(1, msg.api_version)
    }

    @Test
    fun ignoresUnknownFields() {
        val msg = adapter.fromJson("""{"apiVersion":1,"futureField":"x"}""")!!
        assertEquals(1, msg.api_version)
    }

    @Test
    fun binaryRoundTrip() {
        val decoded = GetApiVersionResponse.ADAPTER.decode(
            GetApiVersionResponse(api_version = 1).encode(),
        )
        assertEquals(1, decoded.api_version)
    }

    @Test
    fun serializesCamelCase() {
        val json = adapter.toJson(GetApiVersionResponse(api_version = 1))
        assertTrue("unexpected encoding: $json", json.contains("apiVersion"))
    }
}
