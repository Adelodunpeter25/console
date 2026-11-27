package com.console.mobile.feature.chat.visual

import com.console.mobile.data.model.VisualArtifact
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonClassDiscriminator

/** Render models for workspace image visuals (read_file). */
@Serializable
data class VisualSpec(
    val version: Int,
    val title: String? = null,
    val subtitle: String? = null,
    val blocks: List<VisualBlock> = emptyList(),
)

@OptIn(ExperimentalSerializationApi::class)
@Serializable
@JsonClassDiscriminator("type")
sealed interface VisualBlock {
    @Serializable @SerialName("image")
    data class Image(
        val path: String,
        val alt: String? = null,
    ) : VisualBlock
}

sealed interface VisualParse {
    data class Ok(val spec: VisualSpec) : VisualParse
    data object Unsupported : VisualParse
    data object Invalid : VisualParse
}

private val specJson = Json { ignoreUnknownKeys = true }

const val SUPPORTED_VISUAL_VERSION = 1

fun parseVisual(artifact: VisualArtifact): VisualParse {
    if (artifact.version != SUPPORTED_VISUAL_VERSION) return VisualParse.Unsupported
    return try {
        val spec = specJson.decodeFromJsonElement(VisualSpec.serializer(), artifact.spec)
        if (spec.version != SUPPORTED_VISUAL_VERSION) VisualParse.Unsupported else VisualParse.Ok(spec)
    } catch (_: Exception) {
        VisualParse.Invalid
    }
}
