package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import console.v1.FileSearchResult
import console.v1.SessionHeader
import console.v1.SlashCommandInfo
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** Composer autocomplete data sources — slash commands and `@`-mention file search. */
class AssistRepository(private val api: ConsoleApi) {
    suspend fun listSlashCommands(sessionId: String): List<SlashCommandInfo> = withContext(Dispatchers.IO) {
        try { api.listSlashCommands(sessionId) } catch (_: Exception) { emptyList() }
    }

    suspend fun searchMentionFiles(sessionId: String, query: String, root: String?): List<FileSearchResult> = withContext(Dispatchers.IO) {
        try {
            api.assistSearchFiles(sessionId, query, root).items.sortedWith(
                compareByDescending<FileSearchResult> { it.is_dir }.thenBy { it.relative_path }
            )
        } catch (_: Exception) { emptyList() }
    }

    /**
     * Same-project chats for `$` chat-ref suggestions, excluding the current
     * chat. The scope comes from the current session's own header (project,
     * falling back to cwd), matching the server's ResolveReference scope.
     */
    suspend fun listChatRefs(sessionId: String): List<SessionHeader> = withContext(Dispatchers.IO) {
        try {
            val header = try {
                api.getSession(sessionId, limit = 0).header
            } catch (_: Exception) {
                null
            }
            val siblings = if (header != null && !header.project_id.isNullOrBlank()) {
                api.getSessions(projectId = header.project_id)
            } else if (header != null && !header.cwd.isNullOrBlank()) {
                api.getSessions(cwd = header.cwd)
            } else {
                api.getSessions()
            }
            siblings.filter { it.id != sessionId }
        } catch (_: Exception) {
            emptyList()
        }
    }
}
