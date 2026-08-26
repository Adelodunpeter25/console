package expo.modules.nativestream

import androidx.core.os.bundleOf
import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
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

class NativeStreamModule : Module() {
  private val client: OkHttpClient = OkHttpClient.Builder()
    .connectTimeout(30, TimeUnit.SECONDS)
    .readTimeout(0, TimeUnit.MILLISECONDS) // Indefinite read for SSE streams
    .writeTimeout(30, TimeUnit.SECONDS)
    .retryOnConnectionFailure(true)
    .build()

  private val activeStreams = ConcurrentHashMap<String, Call>()

  override fun definition() = ModuleDefinition {
    Name("NativeStreamModule")

    Events(
      "onStreamEvent",
      "onStreamError",
      "onStreamEnd",
      "onNotificationEvent",
      "onNotificationError"
    )

    AsyncFunction("startChatStream") { streamId: String, url: String, jsonBody: String, headers: Map<String, String>? ->
      activeStreams[streamId]?.cancel()

      val mediaType = "application/json; charset=utf-8".toMediaType()
      val body = jsonBody.toRequestBody(mediaType)

      val requestBuilder = Request.Builder()
        .url(url)
        .post(body)
        .addHeader("Accept", "text/event-stream")
        .addHeader("Cache-Control", "no-cache")

      headers?.forEach { (k, v) ->
        requestBuilder.addHeader(k, v)
      }

      val call = client.newCall(requestBuilder.build())
      activeStreams[streamId] = call

      call.enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
          activeStreams.remove(streamId)
          if (call.isCanceled()) {
            sendEvent(
              "onStreamEnd",
              bundleOf(
                "streamId" to streamId,
                "aborted" to true
              )
            )
            return
          }

          sendEvent(
            "onStreamError",
            bundleOf(
              "streamId" to streamId,
              "error" to (e.message ?: "Network error")
            )
          )
        }

        override fun onResponse(call: Call, response: Response) {
          if (!response.isSuccessful) {
            activeStreams.remove(streamId)
            val errorBody = response.body?.string() ?: ""
            sendEvent(
              "onStreamError",
              bundleOf(
                "streamId" to streamId,
                "statusCode" to response.code,
                "error" to "Server returned HTTP ${response.code}: $errorBody"
              )
            )
            response.close()
            return
          }

          val responseBody = response.body
          if (responseBody == null) {
            activeStreams.remove(streamId)
            sendEvent(
              "onStreamError",
              bundleOf(
                "streamId" to streamId,
                "error" to "Empty response body"
              )
            )
            return
          }

          try {
            val reader = BufferedReader(InputStreamReader(responseBody.byteStream(), Charsets.UTF_8))
            var line: String?
            var isAborted = false
            // Last SSE "id:" seen; sent alongside each data frame so clients
            // can resume with ?since=<seq> after a disconnect.
            var lastId: String? = null

            while (reader.readLine().also { line = it } != null) {
              if (call.isCanceled()) {
                isAborted = true
                break
              }

              val currentLine = line?.trim() ?: continue
              if (currentLine.isEmpty()) continue
              if (currentLine.startsWith("id:")) {
                lastId = currentLine.substring(3).trim()
                continue
              }
              if (!currentLine.startsWith("data:")) continue

              val jsonString = currentLine.substring(5).trim()
              if (jsonString.isEmpty()) continue

              val payload = bundleOf(
                "streamId" to streamId,
                "rawJson" to jsonString
              )
              if (lastId != null) payload.putString("seq", lastId)
              sendEvent("onStreamEvent", payload)
            }

            activeStreams.remove(streamId)
            sendEvent(
              "onStreamEnd",
              bundleOf(
                "streamId" to streamId,
                "aborted" to isAborted
              )
            )
          } catch (e: Exception) {
            activeStreams.remove(streamId)
            if (!call.isCanceled()) {
              sendEvent(
                "onStreamError",
                bundleOf(
                  "streamId" to streamId,
                  "error" to (e.message ?: "Stream reading error")
                )
              )
            }
          } finally {
            response.close()
          }
        }
      })

      true
    }

    AsyncFunction("startNotificationStream") { streamId: String, url: String, headers: Map<String, String>? ->
      activeStreams[streamId]?.cancel()

      val requestBuilder = Request.Builder()
        .url(url)
        .get()
        .addHeader("Accept", "text/event-stream")
        .addHeader("Cache-Control", "no-cache")

      headers?.forEach { (k, v) ->
        requestBuilder.addHeader(k, v)
      }

      val call = client.newCall(requestBuilder.build())
      activeStreams[streamId] = call

      call.enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
          activeStreams.remove(streamId)
          if (!call.isCanceled()) {
            sendEvent(
              "onNotificationError",
              bundleOf(
                "streamId" to streamId,
                "error" to (e.message ?: "Notification stream failed")
              )
            )
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

              sendEvent(
                "onNotificationEvent",
                bundleOf(
                  "streamId" to streamId,
                  "rawJson" to jsonString
                )
              )
            }
          } catch (e: Exception) {
            // ignore on cancel
          } finally {
            activeStreams.remove(streamId)
            response.close()
          }
        }
      })

      true
    }

    /**
     * Generic GET SSE stream (used for run re-attach). Emits the same
     * onStreamEvent/onStreamError/onStreamEnd events as startChatStream,
     * including SSE "id:" passthrough as "seq".
     */
    AsyncFunction("startGetStream") { streamId: String, url: String, headers: Map<String, String>? ->
      activeStreams[streamId]?.cancel()

      val requestBuilder = Request.Builder()
        .url(url)
        .get()
        .addHeader("Accept", "text/event-stream")
        .addHeader("Cache-Control", "no-cache")

      headers?.forEach { (k, v) ->
        requestBuilder.addHeader(k, v)
      }

      val call = client.newCall(requestBuilder.build())
      activeStreams[streamId] = call

      call.enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
          activeStreams.remove(streamId)
          if (call.isCanceled()) {
            sendEvent(
              "onStreamEnd",
              bundleOf(
                "streamId" to streamId,
                "aborted" to true
              )
            )
            return
          }
          sendEvent(
            "onStreamError",
            bundleOf(
              "streamId" to streamId,
              "error" to (e.message ?: "Network error")
            )
          )
        }

        override fun onResponse(call: Call, response: Response) {
          if (!response.isSuccessful) {
            activeStreams.remove(streamId)
            val errorBody = response.body?.string() ?: ""
            sendEvent(
              "onStreamError",
              bundleOf(
                "streamId" to streamId,
                "statusCode" to response.code,
                "error" to "Server returned HTTP ${response.code}: $errorBody"
              )
            )
            response.close()
            return
          }

          val responseBody = response.body
          if (responseBody == null) {
            activeStreams.remove(streamId)
            sendEvent(
              "onStreamError",
              bundleOf(
                "streamId" to streamId,
                "error" to "Empty response body"
              )
            )
            return
          }

          try {
            val reader = BufferedReader(InputStreamReader(responseBody.byteStream(), Charsets.UTF_8))
            var line: String?
            var isAborted = false
            var lastId: String? = null

            while (reader.readLine().also { line = it } != null) {
              if (call.isCanceled()) {
                isAborted = true
                break
              }

              val currentLine = line?.trim() ?: continue
              if (currentLine.isEmpty()) continue
              if (currentLine.startsWith(":")) continue // heartbeat comment
              if (currentLine.startsWith("id:")) {
                lastId = currentLine.substring(3).trim()
                continue
              }
              if (!currentLine.startsWith("data:")) continue

              val jsonString = currentLine.substring(5).trim()
              if (jsonString.isEmpty()) continue

              val payload = bundleOf(
                "streamId" to streamId,
                "rawJson" to jsonString
              )
              if (lastId != null) payload.putString("seq", lastId)
              sendEvent("onStreamEvent", payload)
            }

            activeStreams.remove(streamId)
            sendEvent(
              "onStreamEnd",
              bundleOf(
                "streamId" to streamId,
                "aborted" to isAborted
              )
            )
          } catch (e: Exception) {
            activeStreams.remove(streamId)
            if (!call.isCanceled()) {
              sendEvent(
                "onStreamError",
                bundleOf(
                  "streamId" to streamId,
                  "error" to (e.message ?: "Stream reading error")
                )
              )
            }
          } finally {
            response.close()
          }
        }
      })

      true
    }

    AsyncFunction("abortStream") { streamId: String ->
      val call = activeStreams.remove(streamId)
      if (call != null) {
        call.cancel()
        true
      } else {
        false
      }
    }

    AsyncFunction("isStreamActive") { streamId: String ->
      val call = activeStreams[streamId]
      call != null && !call.isCanceled()
    }
  }
}
