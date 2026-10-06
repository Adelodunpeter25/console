package com.console.mobile.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class ApiResponse<T>(
    val success: Boolean,
    val data: T? = null,
    val error: String? = null,
)

@Serializable
data class CreateSessionDto(
    val cwd: String? = null,
    val projectId: String? = null,
    val modelId: String? = null,
    val provider: String? = null,
    val title: String? = null,
    val approvalMode: String? = null,
)

@Serializable
data class UpdateSessionDto(
    val title: String? = null,
    val cwd: String? = null,
    val projectId: String? = null,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
)

@Serializable
data class RunPromptDto(
    val prompt: String,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    val attachments: List<ImageAttachment> = emptyList(),
    val contextFiles: List<String> = emptyList(),
)

/**
 * An image staged in the composer draft.
 *
 * Held as raw bytes rather than a base64 [String]: base64 costs an extra 33% of
 * the image size, and every thumbnail would decode it straight back out again.
 * [ImageAttachmentSerializer] produces the base64 form only at the two
 * boundaries — the `/api` request body and the draft cache in SharedPreferences —
 * so both keep the exact `{"data", "mimeType"}` shape the server expects and
 * the persisted cache stays readable.
 *
 * Identity equality (a ByteArray compares by reference anyway): the composer
 * strip and the preview dialog both rely on the instance in the list being the
 * same one the user tapped.
 */
@Serializable(with = ImageAttachmentSerializer::class)
class ImageAttachment(
    val id: String,
    val bytes: ByteArray,
    val mimeType: String,
)

/** The on-the-wire / on-disk shape of an [ImageAttachment]. */
@Serializable
private data class ImageAttachmentWire(val data: String, val mimeType: String)

object ImageAttachmentSerializer : kotlinx.serialization.KSerializer<ImageAttachment> {
    private val wire = ImageAttachmentWire.serializer()

    override val descriptor: kotlinx.serialization.descriptors.SerialDescriptor get() = wire.descriptor

    override fun serialize(encoder: kotlinx.serialization.encoding.Encoder, value: ImageAttachment) =
        wire.serialize(encoder, ImageAttachmentWire(encodeBase64(value.bytes), value.mimeType))

    override fun deserialize(decoder: kotlinx.serialization.encoding.Decoder): ImageAttachment {
        val w = wire.deserialize(decoder)
        return ImageAttachment(newAttachmentId(), decodeBase64(w.data), w.mimeType)
    }
}

private var attachmentSeq = 0L

internal fun newAttachmentId(): String = "att-${++attachmentSeq}"

internal fun encodeBase64(bytes: ByteArray): String =
    android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP)

/** Returns an empty array for undecodable input rather than throwing, so one
 * corrupt cached attachment cannot drop the whole draft. */
internal fun decodeBase64(data: String): ByteArray = try {
    if (data.isBlank()) ByteArray(0) else android.util.Base64.decode(data, android.util.Base64.DEFAULT)
} catch (_: Exception) {
    ByteArray(0)
}

@Serializable
data class QueuedPrompt(
    val id: String,
    val sessionId: String,
    val prompt: String,
    val attachments: List<ImageAttachment> = emptyList(),
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    val createdAt: String,
)

@Serializable
data class OAuthLoginUrlDto(val provider: String)

@Serializable
data class OAuthCallbackDto(val provider: String, val code: String, val state: String? = null)

@Serializable
data class AnswerQuestionDto(val requestId: String, val answer: JsonElement)

@Serializable
data class ApproveToolPermissionDto(val requestId: String, val allow: Boolean)

@Serializable
data class SlashCommandInfo(val name: String, val description: String, val builtin: Boolean)

// Items are the shared Wire type (console.v1); the wrapper itself stays
// hand-written until the assist domain migrates, decoded manually in
// OkHttpConsoleApi.assistSearchFiles. Deliberately NOT @Serializable:
// kotlinx cannot serialize Wire types, so manual decoding keeps misuse a
// compile error instead of a runtime one.
data class FileSearchResponse(
    val root: String,
    val query: String,
    val items: List<console.v1.FileSearchResult> = emptyList(),
)

// ProjectInfo moved to the shared protobuf schema (console.v1 from
// proto/console/v1): timestamps now arrive as protojson strings, so the
// hand-written data class with Long timestamps was deleted.

@Serializable
data class ProviderAuthStatus(
    val loggedIn: Boolean,
    val email: String? = null,
    val projectId: String? = null,
    val configuredProjectId: String? = null,
)

// Header is the shared Wire type; messages stay hand-written until the
// messages slice migrates. Deliberately NOT @Serializable: decoded manually
// in OkHttpConsoleApi.getSession (envelope + Moshi header + kotlinx messages).
data class SessionDetailResponse(
    val header: console.v1.SessionHeader,
    val messages: List<AgentMessage> = emptyList(),
    val hasMore: Boolean = false,
    val nextCursor: Long? = null,
)

// FsTreeEntry moved to the shared protobuf schema (console.v1 from
// proto/console/v1): sizes now arrive as protojson strings, and the
// never-populated git status is gone. Unset sizes decode as absent.

// Git working-tree types moved to the shared protobuf schema (console.v1
// from proto/console/v1): status codes stay plain strings, numstat counts
// narrow to uint32 so they stay JSON numbers.


@Serializable
data class FsChangeEvent(val type: String = "fsChange", val projectPath: String, val eventPath: String? = null)
