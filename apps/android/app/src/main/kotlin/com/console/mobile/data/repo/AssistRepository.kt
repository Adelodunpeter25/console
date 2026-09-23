package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.model.FileSearchResult
import com.console.mobile.data.model.SlashCommandInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Composer autocomplete data sources — slash commands and `@`-mention file search. */
class AssistRepository(private val api: ConsoleApi) {
    suspend fun listSlashCommands(sessionId: String): List<SlashCommandInfo> = withContext(Dispatchers.IO) {
        try { api.listSlashCommands(sessionId) } catch (_: Exception) { emptyList() }
    }

    suspend fun searchMentionFiles(sessionId: String, query: String, root: String?): List<FileSearchResult> = withContext(Dispatchers.IO) {
        try { api.assistSearchFiles(sessionId, query, root).items } catch (_: Exception) { emptyList() }
    }
}
