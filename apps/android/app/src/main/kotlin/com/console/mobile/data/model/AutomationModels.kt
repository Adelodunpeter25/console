package com.console.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class Automation(
    val id: String,
    val sessionId: String,
    val prompt: String,
    val kind: String,
    val intervalSeconds: Long = 0,
    val runAt: Long = 0,
    val nextRunAt: Long = 0,
    val status: String,
    val lastFiredAt: Long = 0,
    val lastError: String = "",
    val createdAt: Long = 0,
)

@Serializable
data class AutomationEvent(
    val type: String,
    val automation: Automation? = null,
    val id: String? = null,
    val sessionId: String? = null,
)

@Serializable
data class WebhookSubscription(
    val id: String,
    val sessionId: String,
    val topic: String,
    val secret: String = "",
    val filter: JsonElement? = null,
    val prompt: String,
    val once: Boolean = true,
    val status: String,
    val expiresAt: Long = 0,
    val lastFiredAt: Long = 0,
    val fireCount: Long = 0,
    val lastError: String = "",
    val createdAt: Long = 0,
    val webhookPath: String = "",
    val topicPath: String = "",
    val webhookUrl: String? = null,
    val webhookUrlWithToken: String? = null,
)

@Serializable
data class WebhookEvent(
    val type: String,
    val subscription: WebhookSubscription? = null,
    val id: String? = null,
    val sessionId: String? = null,
)
