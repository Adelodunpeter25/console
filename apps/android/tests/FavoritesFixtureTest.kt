package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ModelFavorite
import console.v1.SetFavoriteResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the favorites migration
 * (docs/plan/shared-protobuf-schema.md §8). Fixtures live in
 * proto/testdata/favorites and are shared by Go, Rust, and Kotlin.
 */
class FavoritesFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<ModelFavorite>>(
        Types.newParameterizedType(List::class.java, ModelFavorite::class.java),
    )
    private val setResponseAdapter = moshi.adapter(SetFavoriteResponse::class.java)
    private val favoriteAdapter = moshi.adapter(ModelFavorite::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/favorites/$name").readText().trim()

    @Test
    fun decodesGoldenListFixture() {
        val items = listAdapter.fromJson(fixture("list.json"))!!
        assertEquals(1, items.size)
        assertEquals("anthropic", items[0].provider)
        assertEquals("claude-opus-4", items[0].model_id)
    }

    @Test
    fun decodesGoldenSetResponseFixture() {
        val msg = setResponseAdapter.fromJson(fixture("set_response.json"))!!
        assertEquals("anthropic", msg.provider)
        assertEquals("claude-opus-4", msg.model_id)
        assertEquals(true, msg.favorite)
    }

    @Test
    fun ignoresUnknownFields() {
        val items = listAdapter.fromJson("[{\"provider\":\"a\",\"modelId\":\"b\",\"futureField\":\"x\"}]")!!
        assertEquals("a", items[0].provider)
    }

    @Test
    fun serializesCamelCase() {
        val encoded = favoriteAdapter.toJson(ModelFavorite(provider = "a", model_id = "b"))
        assertTrue("unexpected encoding: $encoded", encoded.contains("modelId"))
    }
}
