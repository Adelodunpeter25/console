package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.ConsoleApiClient
import console.v1.GitBranchesResponse
import console.v1.GitStatusSummary
import console.v1.ProjectInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient

class GitRepository(
    private val api: ConsoleApi,
    private val apiClient: ConsoleApiClient,
    private val httpClient: OkHttpClient,
) {

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
        if (projects.isEmpty()) return@withContext emptyMap()
        coroutineScope {
            projects.map { project ->
                async {
                    try {
                        val status = api.getGitStatus(project.path)
                        if (status != null && status.branch.isNotBlank()) {
                            project.id to status.branch
                        } else {
                            null
                        }
                    } catch (_: Exception) {
                        null
                    }
                }
            }.awaitAll().filterNotNull().toMap()
        }
    }
}
