package com.console.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
sealed interface MessageContent

@Serializable
@SerialName("text")
data class TextPart(
    val text: String,
    val thoughtSignature: String? = null,
) : MessageContent

@Serializable
@SerialName("thinking")
data class ThinkingPart(val text: String) : MessageContent

@Serializable
@SerialName("toolCall")
data class ToolCallPart(val call: ToolCall) : MessageContent

@Serializable
@SerialName("image")
data class ImagePart(
    val data: String,
    val mimeType: String,
) : MessageContent

@Serializable
sealed interface AgentMessage {
    val id: String?
    val createdAt: Long?
}

@Serializable
@SerialName("user")
data class UserMessage(
    override val id: String? = null,
    override val createdAt: Long? = null,
    val content: String,
    val attachments: List<ImagePart> = emptyList(),
) : AgentMessage

@Serializable
@SerialName("assistant")
data class AssistantMessage(
    override val id: String? = null,
    override val createdAt: Long? = null,
    val content: List<MessageContent> = emptyList(),
    val stopReason: String? = null,
) : AgentMessage

@Serializable
@SerialName("toolResult")
data class ToolResultMessage(
    override val id: String? = null,
    override val createdAt: Long? = null,
    val results: List<ToolResult> = emptyList(),
) : AgentMessage
