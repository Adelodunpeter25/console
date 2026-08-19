package com.console.mobile.networking

import android.os.Handler
import android.os.Looper
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import com.facebook.react.bridge.ReadableMap
import com.facebook.react.bridge.WritableMap
import com.facebook.react.modules.core.DeviceEventManagerModule
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

class NativeStreamModule(private val reactContext: ReactApplicationContext) :
    ReactContextBaseJavaModule(reactContext) {

    override fun getName(): String = "NativeStreamModule"

    private val client: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // Indefinite read for SSE streams
        .writeTimeout(30, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()

    // Track active HTTP/SSE calls by a unique streamId
    private val activeStreams = ConcurrentHashMap<String, Call>()

    private fun sendEvent(eventName: String, params: WritableMap) {
        if (reactContext.hasActiveReactInstance()) {
            reactContext
                .getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java)
                ?.emit(eventName, params)
        }
    }

    /**
     * Starts an SSE stream for chat agent run.
     * Parses `data: <json>` frames in background thread and emits native events to JS.
     */
    @ReactMethod
    fun startChatStream(
        streamId: String,
        url: String,
        jsonBody: String,
        headers: ReadableMap?,
        promise: Promise
    ) {
        try {
            // Cancel any existing stream with the same ID
            activeStreams[streamId]?.cancel()

            val mediaType = "application/json; charset=utf-8".toMediaType()
            val body = jsonBody.toRequestBody(mediaType)

            val requestBuilder = Request.Builder()
                .url(url)
                .post(body)
                .addHeader("Accept", "text/event-stream")
                .addHeader("Cache-Control", "no-cache")

            headers?.let { map ->
                val iterator = map.keySetIterator()
                while (iterator.hasNextKey()) {
                    val key = iterator.nextKey()
                    val value = map.getString(key)
                    if (value != null) {
                        requestBuilder.addHeader(key, value)
                    }
                }
            }

            val call = client.newCall(requestBuilder.build())
            activeStreams[streamId] = call

            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    activeStreams.remove(streamId)
                    if (call.isCanceled()) {
                        val params = Arguments.createMap().apply {
                            putString("streamId", streamId)
                            putBoolean("aborted", true)
                        }
                        sendEvent("onStreamEnd", params)
                        return
                    }

                    val params = Arguments.createMap().apply {
                        putString("streamId", streamId)
                        putString("error", e.message ?: "Network error")
                    }
                    sendEvent("onStreamError", params)
                }

                override fun onResponse(call: Call, response: Response) {
                    if (!response.isSuccessful) {
                        activeStreams.remove(streamId)
                        val errorBody = response.body?.string() ?: ""
                        val params = Arguments.createMap().apply {
                            putString("streamId", streamId)
                            putInt("statusCode", response.code)
                            putString("error", "Server returned HTTP ${response.code}: $errorBody")
                        }
                        sendEvent("onStreamError", params)
                        response.close()
                        return
                    }

                    val responseBody = response.body
                    if (responseBody == null) {
                        activeStreams.remove(streamId)
                        val params = Arguments.createMap().apply {
                            putString("streamId", streamId)
                            putString("error", "Empty response body")
                        }
                        sendEvent("onStreamError", params)
                        return
                    }

                    try {
                        val reader = BufferedReader(InputStreamReader(responseBody.byteStream(), Charsets.UTF_8))
                        var line: String?
                        var isAborted = false

                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) {
                                isAborted = true
                                break
                            }

                            val currentLine = line?.trim() ?: continue
                            if (currentLine.isEmpty() || !currentLine.startsWith("data:")) {
                                continue
                            }

                            val jsonString = currentLine.substring(5).trim()
                            if (jsonString.isEmpty()) continue

                            // Validate JSON frame and dispatch to JS
                            val params = Arguments.createMap().apply {
                                putString("streamId", streamId)
                                putString("rawJson", jsonString)
                            }
                            sendEvent("onStreamEvent", params)
                        }

                        activeStreams.remove(streamId)
                        val endParams = Arguments.createMap().apply {
                            putString("streamId", streamId)
                            putBoolean("aborted", isAborted)
                        }
                        sendEvent("onStreamEnd", endParams)
                    } catch (e: Exception) {
                        activeStreams.remove(streamId)
                        if (!call.isCanceled()) {
                            val params = Arguments.createMap().apply {
                                putString("streamId", streamId)
                                putString("error", e.message ?: "Stream reading error")
                            }
                            sendEvent("onStreamError", params)
                        }
                    } finally {
                        response.close()
                    }
                }
            })

            promise.resolve(true)
        } catch (e: Exception) {
            activeStreams.remove(streamId)
            promise.reject("STREAM_ERROR", e.message, e)
        }
    }

    /**
     * Subscribes to a GET SSE notification stream with auto-reconnection capability.
     */
    @ReactMethod
    fun startNotificationStream(
        streamId: String,
        url: String,
        headers: ReadableMap?,
        promise: Promise
    ) {
        try {
            activeStreams[streamId]?.cancel()

            val requestBuilder = Request.Builder()
                .url(url)
                .get()
                .addHeader("Accept", "text/event-stream")
                .addHeader("Cache-Control", "no-cache")

            headers?.let { map ->
                val iterator = map.keySetIterator()
                while (iterator.hasNextKey()) {
                    val key = iterator.nextKey()
                    val value = map.getString(key)
                    if (value != null) {
                        requestBuilder.addHeader(key, value)
                    }
                }
            }

            val call = client.newCall(requestBuilder.build())
            activeStreams[streamId] = call

            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    activeStreams.remove(streamId)
                    if (!call.isCanceled()) {
                        val params = Arguments.createMap().apply {
                            putString("streamId", streamId)
                            putString("error", e.message ?: "Notification stream failed")
                        }
                        sendEvent("onNotificationError", params)
                    }
                }

                override fun onResponse(call: Call, response: Response) {
                    if (!response.isSuccessful) {
                        activeStreams.remove(streamId)
                        response.close()
                        return
                    }

                    val responseBody = response.body ?: return
                    try {
                        val reader = BufferedReader(InputStreamReader(responseBody.byteStream(), Charsets.UTF_8))
                        var line: String?

                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) break

                            val currentLine = line?.trim() ?: continue
                            if (currentLine.isEmpty() || !currentLine.startsWith("data:")) {
                                continue
                            }

                            val jsonString = currentLine.substring(5).trim()
                            if (jsonString.isEmpty()) continue

                            val params = Arguments.createMap().apply {
                                putString("streamId", streamId)
                                putString("rawJson", jsonString)
                            }
                            sendEvent("onNotificationEvent", params)
                        }
                    } catch (e: Exception) {
                        // ignore error on cancel
                    } finally {
                        activeStreams.remove(streamId)
                        response.close()
                    }
                }
            })

            promise.resolve(true)
        } catch (e: Exception) {
            promise.reject("STREAM_ERROR", e.message, e)
        }
    }

    /**
     * Abort any active stream by its stream ID.
     */
    @ReactMethod
    fun abortStream(streamId: String, promise: Promise) {
        val call = activeStreams.remove(streamId)
        if (call != null) {
            call.cancel()
            promise.resolve(true)
        } else {
            promise.resolve(false)
        }
    }

    /**
     * Check if a stream is actively running.
     */
    @ReactMethod
    fun isStreamActive(streamId: String, promise: Promise) {
        val call = activeStreams[streamId]
        promise.resolve(call != null && !call.isCanceled())
    }

    @ReactMethod
    fun addListener(eventName: String) {
        // Required for React Native event emitter
    }

    @ReactMethod
    fun removeListeners(count: Int) {
        // Required for React Native event emitter
    }
}
