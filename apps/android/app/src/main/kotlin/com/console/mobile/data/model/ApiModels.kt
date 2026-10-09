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
    /** Send an explicit `projectId: null` — the server moves the chat to "No project". */
    @kotlinx.serialization.Transient val clearProject: Boolean = false,
    val title: String? = null,
    val cwd: String? = null,
    val projectId: String? = null,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    val thinkingLevel: String? = null,
)

@Serializable
data class RunPromptDto(
    val prompt: String,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    // The run request carries its own level; the server does not fall back to
    // the one saved on the session, so the picker's choice has to ride along.
    val thinkingLevel: String? = null,
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

// QueuedPrompt moved to the shared protobuf schema (console.v1.QueuedPrompt
// from proto/console/v1): attachments/annotations reuse the message schema,
// created_at stays the server's RFC3339 string, optionals stay absent.

@Serializable
data class OAuthLoginUrlDto(val provider: String)

/** Body of POST /api/auth/github/pat. The token is sent once and never kept. */
@Serializable
data class GitHubTokenDto(val token: String)

@Serializable
data class OAuthCallbackDto(val provider: String, val code: String, val state: String? = null)

@Serializable
data class AnswerQuestionDto(val requestId: String, val answer: JsonElement)

@Serializable
data class ApproveToolPermissionDto(val requestId: String, val allow: Boolean)

// SlashCommandInfo and FileSearchResponse moved to the shared protobuf schema
// (console.v1 from proto/console/v1/assist.proto): SlashCommandInfo is now
// console.v1.SlashCommandInfo and the search envelope console.v1.AssistFileSearchResponse.
// Wire types are decoded through WireJsonAdapterFactory in OkHttpConsoleApi, not
// kotlinx, so these deliberately stay non-@Serializable.

// ProjectInfo moved to the shared protobuf schema (console.v1 from
// proto/console/v1): timestamps now arrive as protojson strings, so the
// hand-written data class with Long timestamps was deleted.

// ProviderAuthStatus moved to the shared protobuf schema (console.v1).

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
