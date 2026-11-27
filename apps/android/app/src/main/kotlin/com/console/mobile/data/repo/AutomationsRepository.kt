package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.Automation
import com.console.mobile.data.model.AutomationEvent
import com.console.mobile.data.model.WebhookEvent
import com.console.mobile.data.model.WebhookSubscription
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response

class AutomationsRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private var streamJob: Job? = null

    private val _automations = MutableStateFlow<List<Automation>>(emptyList())
    val automations: StateFlow<List<Automation>> = _automations.asStateFlow()

    private val _webhooks = MutableStateFlow<List<WebhookSubscription>>(emptyList())
    val webhooks: StateFlow<List<WebhookSubscription>> = _webhooks.asStateFlow()

    fun start() {
        streamJob?.cancel()
        streamJob = scope.launch {
            launch { watchAutomationsLoop() }
            launch { watchWebhooksLoop() }
        }
    }

    fun restart() {
        _automations.value = emptyList()
        _webhooks.value = emptyList()
        start()
    }

    private suspend fun watchAutomationsLoop() {
        var backoffMs = 1_000L
        while (scope.isActive) {
            try {
                sseEvents("/api/automations/stream").collect { (eventName, data) ->
                    when (eventName) {
                        "automations" -> {
                            val list = ConsoleJson.decodeFromString(ListSerializer(Automation.serializer()), data)
                            _automations.value = list
                        }
                        "automation" -> {
                            val ev = ConsoleJson.decodeFromString(AutomationEvent.serializer(), data)
                            applyAutomationEvent(ev)
                        }
                    }
                }
                backoffMs = 1_000L
            } catch (e: Exception) {
                if (e is CancellationException) throw e
            }
            delay(backoffMs)
            backoffMs = (backoffMs * 2).coerceAtMost(30_000L)
        }
    }

    private suspend fun watchWebhooksLoop() {
        var backoffMs = 1_000L
        while (scope.isActive) {
            try {
                sseEvents("/api/webhooks/stream").collect { (eventName, data) ->
                    when (eventName) {
                        "webhooks" -> {
                            val list = ConsoleJson.decodeFromString(ListSerializer(WebhookSubscription.serializer()), data)
                            _webhooks.value = list
                        }
                        "webhook" -> {
                            val ev = ConsoleJson.decodeFromString(WebhookEvent.serializer(), data)
                            applyWebhookEvent(ev)
                        }
                    }
                }
                backoffMs = 1_000L
            } catch (e: Exception) {
                if (e is CancellationException) throw e
            }
            delay(backoffMs)
            backoffMs = (backoffMs * 2).coerceAtMost(30_000L)
        }
    }

    private fun applyAutomationEvent(ev: AutomationEvent) {
        if (ev.type == "deleted" || (ev.type == "fired" && ev.automation?.status == "done")) {
            val id = ev.id ?: ev.automation?.id ?: return
            _automations.update { list -> list.filterNot { it.id == id } }
            return
        }
        val auto = ev.automation ?: return
        _automations.update { list ->
            if (list.any { it.id == auto.id }) list.map { if (it.id == auto.id) auto else it }
            else list + auto
        }
    }

    private fun applyWebhookEvent(ev: WebhookEvent) {
        if (ev.type == "deleted" || (ev.type == "fired" && ev.subscription?.status == "done")) {
            val id = ev.id ?: ev.subscription?.id ?: return
            _webhooks.update { list -> list.filterNot { it.id == id } }
            return
        }
        val sub = ev.subscription ?: return
        _webhooks.update { list ->
            if (list.any { it.id == sub.id }) list.map { if (it.id == sub.id) sub else it }
            else list + sub
        }
    }

    fun pauseAutomation(id: String) {
        scope.launch {
            runCatching { api.pauseAutomation(id) }.onSuccess { updated ->
                applyAutomationEvent(AutomationEvent(type = "paused", automation = updated, id = id))
            }
        }
    }

    fun resumeAutomation(id: String) {
        scope.launch {
            runCatching { api.resumeAutomation(id) }.onSuccess { updated ->
                applyAutomationEvent(AutomationEvent(type = "resumed", automation = updated, id = id))
            }
        }
    }

    fun deleteAutomation(id: String) {
        scope.launch {
            runCatching { api.deleteAutomation(id) }.onSuccess {
                _automations.update { list -> list.filterNot { it.id == id } }
            }
        }
    }

    fun deleteWebhook(id: String) {
        scope.launch {
            runCatching { api.deleteWebhook(id) }.onSuccess {
                _webhooks.update { list -> list.filterNot { it.id == id } }
            }
        }
    }

    private fun sseEvents(path: String): Flow<Pair<String, String>> = callbackFlow {
        val baseUrl = apiClient.baseUrl.trimEnd('/')
        val reqBuilder = Request.Builder()
            .url("$baseUrl$path")
            .addHeader("Accept", "text/event-stream")
            .addHeader("Cache-Control", "no-cache")
        apiClient.authToken?.let { reqBuilder.addHeader("Authorization", "Bearer $it") }

        val call = httpClient.newCall(reqBuilder.build())
        val job = launch(Dispatchers.IO) {
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    close()
                }

                override fun onResponse(call: Call, response: Response) {
                    if (!response.isSuccessful) {
                        response.close()
                        close()
                        return
                    }
                    val body = response.body
                    if (body == null) {
                        response.close()
                        close()
                        return
                    }
                    try {
                        val reader = BufferedReader(InputStreamReader(body.byteStream(), Charsets.UTF_8))
                        var currentEvent = ""
                        val dataBuf = StringBuilder()
                        var line: String?
                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) break
                            val rawLine = line ?: continue
                            if (rawLine.isEmpty()) {
                                if (currentEvent.isNotEmpty() && dataBuf.isNotEmpty()) {
                                    trySend(currentEvent to dataBuf.toString())
                                }
                                currentEvent = ""
                                dataBuf.setLength(0)
                                continue
                            }
                            if (rawLine.startsWith(":")) continue
                            if (rawLine.startsWith("event:")) {
                                currentEvent = rawLine.substring(6).trim()
                            } else if (rawLine.startsWith("data:")) {
                                if (dataBuf.isNotEmpty()) dataBuf.append('\n')
                                dataBuf.append(rawLine.substring(5).trim())
                            }
                        }
                    } catch (_: Exception) {
                    } finally {
                        response.close()
                        close()
                    }
                }
            })
        }
        awaitClose {
            job.cancel()
            call.cancel()
        }
    }
}
