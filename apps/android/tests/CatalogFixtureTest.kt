package com.console.mobile

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.Model
import console.v1.ProviderCatalogEntry
import console.v1.ProviderModelsResponse
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * Golden-fixture tests for the provider catalog migration. Fixtures live in
 * proto/testdata/catalog and are shared by Go, Rust, and Kotlin. The catalog
 * envelope stays kotlinx; the entries and models are Wire types.
 */
class CatalogFixtureTest {
    private val moshi: Moshi = Moshi.Builder()
        .add(WireJsonAdapterFactory())
        .build()
    private val listAdapter = moshi.adapter<List<ProviderCatalogEntry>>(
        Types.newParameterizedType(List::class.java, ProviderCatalogEntry::class.java),
    )
    private val modelsAdapter = moshi.adapter(ProviderModelsResponse::class.java)

    private fun fixture(name: String): String =
        File("../../../proto/testdata/catalog/$name").readText().trim()

    @Test
    fun decodesGoldenCatalogFixture() {
        val entries = listAdapter.fromJson(fixture("providers.json"))!!
        assertEquals(2, entries.size)
        val codex = entries[0]
        assertEquals("codex", codex.name)
        assertEquals("Codex", codex.display_name)
        assertEquals("OpenAI", codex.description)
        assertEquals("oauth", codex.auth_method)
        assertEquals(2, codex.models.size)
        val gpt5 = codex.models[0]
        assertEquals("gpt-5", gpt5.id)
        // uint32 counts stay JSON numbers.
        assertEquals(272000, gpt5.context_window)
        assertTrue(gpt5.supports_images)
        assertEquals(listOf("low", "high"), gpt5.supported_thinking_levels)
        assertEquals("medium", gpt5.default_thinking_level)
        // Undeclared optionals are absent, not empty/zero.
        val mini = codex.models[1]
        assertFalse(mini.supports_images)
        assertTrue(mini.supported_thinking_levels.isEmpty())
        assertNull(mini.default_thinking_level)
        // A provider with an omitted model list decodes to empty.
        assertTrue(entries[1].models.isEmpty())
    }

    @Test
    fun decodesGoldenProviderModelsFixture() {
        val resp = modelsAdapter.fromJson(fixture("provider_models.json"))!!
        assertEquals("codex", resp.provider)
        assertEquals(1, resp.models.size)
        assertEquals(272000, resp.models[0].context_window)
    }

    @Test
    fun ignoresUnknownFields() {
        val entry = moshi.adapter(ProviderCatalogEntry::class.java).fromJson(
            "{\"name\":\"p\",\"display_name\":\"P\",\"description\":\"d\",\"auth_method\":\"none\",\"futureField\":1}",
        )!!
        assertEquals("p", entry.name)
        assertTrue(entry.models.isEmpty())
    }

    @Test
    fun modelSerializesCountsAsNumbers() {
        val encoded = moshi.adapter(Model::class.java).toJson(
            Model(id = "gpt-5", provider = "codex", context_window = 272000),
        )
        // Wire's JSON adapter emits protojson camelCase keys, so the wire
        // name is contextWindow even though the Kotlin property is snake_case.
        assertTrue("unexpected encoding: $encoded", encoded.contains("\"contextWindow\":272000"))
        assertFalse("unexpected encoding: $encoded", encoded.contains("supports_images"))
    }
}