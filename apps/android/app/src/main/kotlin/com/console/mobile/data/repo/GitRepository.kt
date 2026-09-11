package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.GitBranchesResponse
import com.console.mobile.data.model.GitStatusSummary
import com.console.mobile.data.model.ProjectInfo
import java.io.BufferedReader
import java.io.InputStreamReader
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.Call
import okhttp3.Callback
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import java.io.IOException

class GitRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
) {
    suspend fun getDiff(repoPath: String, filePath: String? = null): String? = withContext(Dispatchers.IO) {
        api.getDiff(repoPath, filePath)
    }

    suspend fun getStatus(path: String): GitStatusSummary? = withContext(Dispatchers.IO) {
        api.getGitStatus(path)
    }

    suspend fun listBranches(repoPath: String): GitBranchesResponse? = withContext(Dispatchers.IO) {
        api.listBranches(repoPath)
    }

    suspend fun checkoutBranch(repoPath: String, branch: String) = withContext(Dispatchers.IO) {
        api.checkoutBranch(repoPath, branch)
    }

    suspend fun fetchBranchesForProjects(projects: List<ProjectInfo>): Map<String, String> = withContext(Dispatchers.IO) {
        val result = mutableMapOf<String, String>()
        for (project in projects) {
            try {
                val status = api.getGitStatus(project.path)
                if (status != null && status.branch.isNotBlank()) {
                    result[project.id] = status.branch
                }
            } catch (_: Exception) {
            }
        }
        result
    }

    /**
     * Watches git status changes via GET /api/git/status/watch?path=... SSE.
     */
    fun watchStatus(path: String): Flow<GitStatusSummary> = callbackFlow {
        val baseUrl = apiClient.baseUrl.trimEnd('/')
        val encoded = java.net.URLEncoder.encode(path, "UTF-8")
        val url = "$baseUrl/api/git/status/watch?path=$encoded"

        val reqBuilder = Request.Builder()
            .url(url)
            .addHeader("Accept", "text/event-stream")
            .addHeader("Cache-Control", "no-cache")

        apiClient.authToken?.let {
            reqBuilder.addHeader("Authorization", "Bearer $it")
        }

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
                        var eventType = "message"
                        while (reader.readLine().also { line = it } != null) {
                            if (call.isCanceled()) break
                            val l = line?.trim() ?: continue
                            if (l.isEmpty()) continue
                            if (l.startsWith(":")) continue
                            if (l.startsWith("event:")) {
                                eventType = l.substring(6).trim()
                                continue
                            }
                            if (l.startsWith("data:")) {
                                val raw = l.substring(5).trim()
                                if (eventType == "gitStatus" || eventType == "message") {
                                    try {
                                        val summary = ConsoleJson.decodeFromString(GitStatusSummary.serializer(), raw)
                                        trySend(summary)
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
