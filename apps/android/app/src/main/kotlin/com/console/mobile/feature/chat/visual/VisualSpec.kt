package com.console.mobile.feature.chat.visual

import com.console.mobile.data.model.VisualArtifact
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonClassDiscriminator

/** Render models for the v1 visual spec (docs/plan/visualize.md §3). */
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
    @Serializable @SerialName("text")
    data class Text(val text: String) : VisualBlock

    @Serializable @SerialName("metric")
    data class Metric(
        val label: String,
        val value: String,
        val delta: String? = null,
        val trend: String? = null,
        val caption: String? = null,
    ) : VisualBlock

    @Serializable @SerialName("table")
    data class Table(
        val caption: String? = null,
        val columns: List<String>,
        val rows: List<List<String>>,
    ) : VisualBlock

    @Serializable @SerialName("chart")
    data class Chart(
        val kind: String,
        val title: String? = null,
        val xLabel: String? = null,
        val yLabel: String? = null,
        val series: List<VisualSeries>,
    ) : VisualBlock
}

@Serializable
data class VisualSeries(val name: String, val points: List<VisualPoint>)

@Serializable
data class VisualPoint(val x: Double, val y: Double, val label: String? = null)

sealed interface VisualParse {
    data class Ok(val spec: VisualSpec) : VisualParse
    /** A newer spec version than this build understands. */
    data object Unsupported : VisualParse
    data object Invalid : VisualParse
}

private val specJson = Json { ignoreUnknownKeys = true }

const val SUPPORTED_VISUAL_VERSION = 1

/** Version is checked before decoding so a newer spec never half-parses. */
fun parseVisual(artifact: VisualArtifact): VisualParse {
    if (artifact.version != SUPPORTED_VISUAL_VERSION) return VisualParse.Unsupported
    return try {
        val spec = specJson.decodeFromJsonElement(VisualSpec.serializer(), artifact.spec)
        if (spec.version != SUPPORTED_VISUAL_VERSION) VisualParse.Unsupported else VisualParse.Ok(spec)
    } catch (_: Exception) {
        VisualParse.Invalid
    }
}
