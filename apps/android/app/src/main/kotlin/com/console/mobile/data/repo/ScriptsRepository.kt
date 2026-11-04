package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.squareup.moshi.Moshi
import com.squareup.wire.WireJsonAdapterFactory
import console.v1.ProjectScript
import console.v1.ScriptRun
import console.v1.ScriptRunEvent
import java.io.BufferedReader
import java.io.IOException
import java.io.InputStreamReader
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response

/** Everything the scripts sheet needs for one checkout. */
data class ProjectScriptsState(
    val scripts: List<ProjectScript> = emptyList(),
    /** `"console.toml"` or `"missing"`, straight from the list response. */
    val source: String = "",
    val loading: Boolean = false,
    val loaded: Boolean = false,
    val error: String? = null,
)

/** Latest known run for one script row, plus its rendered output tail. */
data class ScriptRunView(
    val run: ScriptRun? = null,
    val output: String = "",
    val starting: Boolean = false,
) {
    /** A run is live while starting, or while its last status says running. */
    val isRunning: Boolean
        get() = starting || run?.status == "running"
}

/** A port the run's output announces, for the row's "Open" affordance. */
private val PORT_PATTERN = Regex("""(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d{2,5})""")

/**
 * Project run scripts (`console.toml` `[scripts]`), scoped per checkout: the
 * server's `cwd` query keeps a worktree chat on its own copy of the scripts,
 * exactly like the desktop's scripts target. Definitions and execution live on
 * the server; this keeps the client's view of them and follows live runs over
 * the run's SSE stream.
 *
 * State is keyed by `projectId + cwd`: `states` holds each checkout's script
 * list, `runs` holds one view per script, so a script left running keeps
 * streaming after the sheet closes and reappears with its output when the
 * sheet is reopened.
 */
class ScriptsRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    private val _states = MutableStateFlow<Map<String, ProjectScriptsState>>(emptyMap())
    val states: StateFlow<Map<String, ProjectScriptsState>> = _states.asStateFlow()

    private val _runs = MutableStateFlow<Map<String, ScriptRunView>>(emptyMap())
    val runs: StateFlow<Map<String, ScriptRunView>> = _runs.asStateFlow()

    /** One live stream per script row; replaced only once it has ended. */
    private val watches = mutableMapOf<String, Job>()

    // ScriptRunEvent is a shared-schema type (console.v1); the stream has no
    // envelope, so the frame decodes straight off the SSE line.
    private val eventAdapter = Moshi.Builder().add(WireJsonAdapterFactory()).build()
        .adapter(ScriptRunEvent::class.java)

    companion object {
        /** Separator for composite keys; never appears in ids or paths. */
        private const val SEP = '\u001f'

        /** Cap on kept output per run, so a chatty dev server cannot grow unbounded. */
        private const val MAX_OUTPUT = 200_000

        fun projectKey(projectId: String, cwd: String?): String = projectId + SEP + cwd.orEmpty()

        /** Every run key of one checkout starts with this prefix. */
        fun projectPrefix(projectId: String, cwd: String?): String = projectKey(projectId, cwd) + SEP

        fun runKey(projectId: String, cwd: String?, scriptId: String): String =
            projectKey(projectId, cwd) + SEP + scriptId

        /**
         * A port announced in [output], when the run looks like a dev server.
         * Localhost first, then any of the usual bind addresses.
         */
        fun portInOutput(output: String): Int? =
            PORT_PATTERN.find(output)?.groupValues?.get(1)?.toIntOrNull()?.takeIf { it in 1..65535 }
    }

    /** The script list state for one checkout, defaulted when unseen. */
    fun stateFor(projectId: String, cwd: String?): ProjectScriptsState =
        _states.value[projectKey(projectId, cwd)] ?: ProjectScriptsState()

    /** The run view for one script, defaulted when never run. */
    fun runViewFor(projectId: String, cwd: String?, scriptId: String): ScriptRunView =
        _runs.value[runKey(projectId, cwd, scriptId)] ?: ScriptRunView()

    /** Live run count for a checkout, for the chat header's indicator. */
    fun runningCount(projectId: String, cwd: String?): Int {
        val prefix = projectKey(projectId, cwd) + SEP
        return _runs.value.count { (key, view) -> key.startsWith(prefix) && view.isRunning }
    }

    /**
     * Re-read the script list and latest runs for a checkout. Follows any run
     * already live (a dev server started before this screen opened).
     */
    suspend fun refresh(projectId: String, cwd: String?) {
        val key = projectKey(projectId, cwd)
        _states.value = _states.value + (key to stateFor(projectId, cwd).copy(loading = true, error = null))
        try {
            val (result, runs) = withContext(Dispatchers.IO) {
                api.getProjectScripts(projectId, cwd) to api.listScriptRuns(projectId, cwd)
            }
            _states.value = _states.value + (
                key to ProjectScriptsState(
                    scripts = result.scripts,
                    source = result.source,
                    loading = false,
                    loaded = true,
                    error = null,
                )
                )
            applyRuns(projectId, cwd, runs)
        } catch (e: Exception) {
            _states.value = _states.value + (
                key to stateFor(projectId, cwd).copy(
                    loading = false,
                    loaded = true,
                    error = e.message ?: "Couldn't load run scripts",
                )
                )
        }
    }

    /**
     * Refresh only the run snapshots — the light call the chat screen makes on
     * entry so the header can say whether this checkout has a script running
     * without loading the whole panel.
     */
    suspend fun refreshRuns(projectId: String, cwd: String?) {
        try {
            val runs = withContext(Dispatchers.IO) { api.listScriptRuns(projectId, cwd) }
            applyRuns(projectId, cwd, runs)
        } catch (_: Exception) {
            // Silent: the badge is an extra, the sheet reports real errors.
        }
    }

    /** Start [scriptId] and follow its run. The command stays server-side. */
    fun run(projectId: String, cwd: String?, scriptId: String) {
        val key = runKey(projectId, cwd, scriptId)
        val existing = _runs.value[key]
        if (existing?.isRunning == true) return
        _runs.value = _runs.value + (key to ScriptRunView(run = existing?.run, output = existing?.output ?: "", starting = true))
        scope.launch {
            try {
                val run = withContext(Dispatchers.IO) { api.startScriptRun(projectId, scriptId, cwd) }
                val output = snapshotOutput(run)
                _runs.value = _runs.value + (key to ScriptRunView(run = run, output = output, starting = false))
                watch(projectId, cwd, scriptId, run.run_id)
            } catch (e: Exception) {
                val current = _runs.value[key] ?: ScriptRunView()
                _runs.value = _runs.value + (key to current.copy(starting = false))
                setError(projectId, cwd, "Couldn't start script: ${e.message ?: "unknown error"}")
            }
        }
    }

    /** Stop the script's latest run; the stream delivers the final status. */
    fun stop(projectId: String, cwd: String?, scriptId: String) {
        val runId = _runs.value[runKey(projectId, cwd, scriptId)]?.run?.run_id ?: return
        scope.launch {
            try {
                withContext(Dispatchers.IO) { api.stopScriptRun(projectId, runId, cwd) }
            } catch (e: Exception) {
                setError(projectId, cwd, "Couldn't stop script: ${e.message ?: "unknown error"}")
            }
        }
    }

    /**
     * Drop every script and run view. Used when the backend changes: streams
     * and run ids belong to the server they came from, so keeping them would
     * show the previous server's state.
     */
    fun clear() {
        watches.values.forEach { it.cancel() }
        watches.clear()
        _states.value = emptyMap()
        _runs.value = emptyMap()
    }

    // --- internals ----------------------------------------------------------

    private fun setError(projectId: String, cwd: String?, message: String) {
        val key = projectKey(projectId, cwd)
        _states.value = _states.value + (key to stateFor(projectId, cwd).copy(error = message))
    }

    /** Merge latest-run snapshots, letting a live stream own its own run. */
    private fun applyRuns(projectId: String, cwd: String?, runs: List<ScriptRun>) {
        for (run in runs) {
            val key = runKey(projectId, cwd, run.script_id)
            val live = watches[key]?.isActive == true
            val owned = live && run.status == "running" &&
                _runs.value[key]?.run?.run_id == run.run_id
            if (owned) continue
            _runs.value = _runs.value + (key to ScriptRunView(run = run, output = snapshotOutput(run), starting = false))
            if (run.status == "running") watch(projectId, cwd, run.script_id, run.run_id)
        }
    }

    /**
     * Follow one run's SSE stream, appending output and applying status
     * frames. When the stream ends (the server closes it on a terminal
     * status, or the connection dropped) the run is reconciled against the
     * retained record, so output between frames and a missed final status
     * cannot leave the row stuck.
     */
    private fun watch(projectId: String, cwd: String?, scriptId: String, runId: String) {
        val key = runKey(projectId, cwd, scriptId)
        if (watches[key]?.isActive == true) return
        watches[key] = scope.launch {
            try {
                events(projectId, runId).collect { event ->
                    when (val payload = event.event) {
                        is ScriptRunEvent.Event.Output -> appendOutput(key, payload.value.text)
                        is ScriptRunEvent.Event.Status -> applyStatus(key, payload.value.status)
                        is ScriptRunEvent.Event.Exit -> applyStatus(key, payload.value.status, payload.value.exit_code)
                        null -> {}
                    }
                }
            } catch (_: Exception) {
                // A dropped stream still reconciles below.
            }
            reconcile(projectId, cwd, scriptId, runId)
        }
    }

    private suspend fun reconcile(projectId: String, cwd: String?, scriptId: String, runId: String) {
        val key = runKey(projectId, cwd, scriptId)
        try {
            val run = withContext(Dispatchers.IO) { api.getScriptRun(projectId, runId, cwd) }
            _runs.value = _runs.value + (key to ScriptRunView(run = run, output = snapshotOutput(run), starting = false))
        } catch (e: Exception) {
            // The server forgot the run (daemon restart): drop the stale view
            // so the row returns to "Not run yet" instead of staying stuck.
            if (e.message?.contains("not found", ignoreCase = true) == true) {
                val current = _runs.value[key]
                if (current?.run?.run_id == runId) {
                    _runs.value = _runs.value + (key to ScriptRunView())
                }
            }
        }
    }

    private fun appendOutput(key: String, text: String) {
        if (text.isEmpty()) return
        val view = _runs.value[key] ?: ScriptRunView()
        val combined = view.output + text
        val output = if (combined.length > MAX_OUTPUT) combined.takeLast(MAX_OUTPUT) else combined
        _runs.value = _runs.value + (key to view.copy(output = output))
    }

    private fun applyStatus(key: String, status: String, exitCode: Int? = null) {
        val view = _runs.value[key] ?: ScriptRunView()
        val run = (view.run ?: ScriptRun()).copy(
            status = status,
            exit_code = exitCode ?: view.run?.exit_code,
        )
        _runs.value = _runs.value + (key to view.copy(run = run))
    }

    private fun snapshotOutput(run: ScriptRun): String {
        val combined = run.stdout + run.stderr
        return if (combined.length > MAX_OUTPUT) combined.takeLast(MAX_OUTPUT) else combined
    }

    /** GET /api/projects/:id/scripts/runs/:runId/stream, as an SSE flow. */    private fun events(projectId: String, runId: String): Flow<ScriptRunEvent> = callbackFlow {
        val baseUrl = apiClient.baseUrl.trimEnd('/')
        val url = "$baseUrl/api/projects/$projectId/scripts/runs/$runId/stream"

        val reqBuilder = Request.Builder()
            .url(url)
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
                        var line: String?
                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) break
                            val l = line?.trim() ?: continue
                            if (l.isEmpty() || l.startsWith(":") || l.startsWith("event:")) continue
                            if (l.startsWith("data:")) {
                                val raw = l.substring(5).trim()
                                if (raw.isNotEmpty()) {
                                    try {
                                        eventAdapter.fromJson(raw)?.let { trySend(it) }
                                    } catch (_: Exception) {
                                    }
                                }
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
