package com.console.mobile.data.api

import kotlinx.serialization.KSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

/**
 * Shared OkHttp plumbing for the Console backend.
 *
 * Mirrors packages/api client semantics: `{ success, data, error }` envelope,
 * `Authorization: Bearer <token>` when present, query params appended like
 * axios `params`. Bodies are pre-encoded JSON strings — callers use
 * [encodeBody] with the model's serializer. Streaming (SSE) stays in
 * ChatStreamClient; this is plain JSON request/response only.
 */
class HttpTransport(
    private val callClient: OkHttpClient,
    private val json: Json = ConsoleJson,
    private val baseUrl: () -> String,
    private val authToken: () -> String?,
) {
    private val jsonMedia = "application/json; charset=utf-8".toMediaType()

    fun url(path: String, params: Map<String, String?> = emptyMap()): String {
        val root = baseUrl().trim().trimEnd('/')
        val base = "$root$path".toHttpUrlOrNull()
            ?: throw ApiException("Invalid backend URL: $root$path")
        val builder = base.newBuilder()
        for ((k, v) in params) {
            if (v != null) builder.addQueryParameter(k, v)
        }
        return builder.build().toString()
    }

    fun <T> encodeBody(serializer: KSerializer<T>, value: T): String =
        json.encodeToString(serializer, value)

    private fun requestBuilder(url: String): Request.Builder {
        val b = Request.Builder().url(url).addHeader("Accept", "application/json")
        val token = authToken()
        if (!token.isNullOrBlank()) b.addHeader("Authorization", "Bearer $token")
        return b
    }

    private fun execute(request: Request): String {
        callClient.newCall(request).execute().use { res ->
            val raw = try { res.body?.string().orEmpty() } catch (_: Exception) { "" }
            if (!res.isSuccessful) {
                throw ApiException(extractError(raw) ?: "Request failed with status ${res.code}", code = res.code.toString())
            }
            return raw
        }
    }

    private fun extractError(raw: String): String? {
        if (raw.isBlank()) return null
        return try {
            val el = json.parseToJsonElement(raw)
            ((el as? JsonObject)?.get("error") as? JsonPrimitive)?.contentOrNull
                ?: raw.take(300)
        } catch (_: Exception) {
            raw.take(300)
        }
    }

    fun get(path: String, params: Map<String, String?> = emptyMap()): String =
        execute(requestBuilder(url(path, params)).get().build())

    fun post(path: String, bodyJson: String? = null): String {
        val body = (bodyJson ?: "").toRequestBody(jsonMedia)
        return execute(requestBuilder(url(path)).post(body).build())
    }

    fun patch(path: String, bodyJson: String? = null): String {
        val body = (bodyJson ?: "").toRequestBody(jsonMedia)
        return execute(requestBuilder(url(path)).patch(body).build())
    }

    fun put(path: String, bodyJson: String? = null): String {
        val body = (bodyJson ?: "").toRequestBody(jsonMedia)
        return execute(requestBuilder(url(path)).put(body).build())
    }

    fun delete(path: String, params: Map<String, String?> = emptyMap(), bodyJson: String? = null): String {
        val b = requestBuilder(url(path, params))
        if (bodyJson != null) b.delete(bodyJson.toRequestBody(jsonMedia)) else b.delete()
        return execute(b.build())
    }

    /** Decode `{ success, data }` — throws ApiException on failure envelope. */
    fun <T> unwrap(raw: String, dataSerializer: KSerializer<T>, action: String): T {
        val root = try {
            json.parseToJsonElement(raw) as? JsonObject
        } catch (_: Exception) {
            null
        } ?: throw ApiException("Failed to $action")
        val success = (root["success"] as? JsonPrimitive)?.booleanOrNull ?: true
        if (!success) {
            throw ApiException((root["error"] as? JsonPrimitive)?.contentOrNull ?: "Failed to $action")
        }
        val data = root["data"] ?: throw ApiException("Failed to $action")
        if (data is JsonPrimitive && data.contentOrNull == null) {
            @Suppress("UNCHECKED_CAST")
            return null as T
        }
        return json.decodeFromJsonElement(dataSerializer, data)
    }

    /** Decode envelope-or-raw payloads (fs/assist return `data ?? body`). */
    fun <T> unwrapOrRaw(raw: String, dataSerializer: KSerializer<T>, action: String): T {
        val root = try {
            json.parseToJsonElement(raw)
        } catch (_: Exception) {
            throw ApiException("Failed to $action")
        }
        if (root is JsonObject && root.containsKey("success") && root.containsKey("data")) {
            val success = (root["success"] as? JsonPrimitive)?.booleanOrNull ?: true
            if (!success) {
                throw ApiException((root["error"] as? JsonPrimitive)?.contentOrNull ?: "Failed to $action")
            }
            val data = root["data"]!!
            if (data is JsonPrimitive && data.contentOrNull == null) {
                @Suppress("UNCHECKED_CAST")
                return null as T
            }
            return json.decodeFromJsonElement(dataSerializer, data)
        }
        return json.decodeFromJsonElement(dataSerializer, root)
    }
}
