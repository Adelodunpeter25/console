package com.console.mobile.data.api

import kotlinx.serialization.json.Json

val ConsoleJson = Json {
    ignoreUnknownKeys = true
    isLenient = true
    coerceInputValues = true
    explicitNulls = false
}

/** Port of unwrapData in packages/api services. Throws on failure envelope. */
fun <T> unwrapEnvelope(body: Envelope<T>, action: String): T {
    if (body.success == false || body.data == null) {
        throw ApiException(body.error ?: "Failed to $action")
    }
    return body.data
}

data class Envelope<T>(val success: Boolean, val data: T?, val error: String?)

class ApiException(message: String, val code: String? = null) : Exception(message)

/** Parse one SSE frame buffer — port of extractSseFrames in git.service.ts */
data class SseFrame(val event: String, val data: String)

fun extractSseFrames(buffer: String): Pair<List<SseFrame>, String> {
    val frames = mutableListOf<SseFrame>()
    val parts = buffer.split("\n\n")
    val rest = parts.lastOrNull() ?: ""
    for (part in parts.dropLast(1)) {
        var event = "message"
        val dataLines = mutableListOf<String>()
        for (line in part.split("\n")) {
            when {
                line.startsWith("event:") -> event = line.substring(6).trim()
                line.startsWith("data:") -> dataLines.add(line.substring(5).trimStart())
            }
        }
        frames.add(SseFrame(event, dataLines.joinToString("\n")))
    }
    return frames to rest
}
