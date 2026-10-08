package com.console.mobile.core.chat

import com.console.mobile.core.util.folderName
import console.v1.ProjectInfo
import console.v1.SessionHeader
import com.console.mobile.data.model.SessionStatus

fun isDraftSession(state: ChatSessionState): Boolean =
    state.input.trim().isNotEmpty() || state.attachments.isNotEmpty()

fun hasPersistableDraft(state: ChatSessionState): Boolean = isDraftSession(state)

fun draftPreview(state: ChatSessionState, maxLen: Int = 48): String {
    val text = state.input.trim().replace("\\s+".toRegex(), " ")
    if (text.isNotEmpty()) {
        return if (text.length > maxLen) "Draft: ${text.take(maxLen)}…" else "Draft: $text"
    }
    if (state.attachments.isNotEmpty()) {
        val n = state.attachments.size
        return if (n == 1) "Draft: 1 image" else "Draft: $n images"
    }
    return "Draft"
}

fun createEphemeralDraftHeader(
    id: String,
    draft: ChatSessionState,
    projectId: String? = null,
    cwd: String? = null,
): SessionHeader {
    val now = draft.draftUpdatedAt ?: System.currentTimeMillis()
    val cleanTitle = draftPreview(draft, 32).removePrefix("Draft: ").ifBlank { "Draft" }
    return SessionHeader(
        id = id,
        title = cleanTitle,
        cwd = cwd ?: "",
        project_id = projectId,
        model_id = "",
        provider = "",
        created_at = now,
        updated_at = now,
        message_count = 0,
        status = "idle",
    )
}

fun isDraftHeader(session: SessionHeader, state: ChatSessionState?): Boolean {
    if (state == null) return false
    return (session.message_count ?: 0) == 0 && isDraftSession(state)
}

data class GroupedProjectSection(
    val projectId: String?,
    val projectName: String,
    val data: List<SessionHeader>,
    val latestAt: Long,
)

fun formatProjectTitle(name: String): String {
    if (name.isBlank()) return ""
    return name
        .replace("[-_]+".toRegex(), " ")
        .split(" ")
        .filter { it.isNotEmpty() }
        .joinToString(" ") { it.replaceFirstChar { c -> c.uppercaseChar() } }
        .trim()
}

fun buildGroupedProjectSections(
    sessions: List<SessionHeader>,
    projects: List<ProjectInfo>,
    draftSessions: Map<String, ChatSessionState>,
    searchQuery: String = "",
): List<GroupedProjectSection> {
    val q = searchQuery.trim().lowercase()
    val filteredSessions = if (q.isEmpty()) sessions else sessions.filter { it.title.lowercase().contains(q) }

    // Draft summaries for 0-message sessions
    val draftEntries = draftSessions.filter { (_, s) ->
        s.messages.isEmpty() && isDraftSession(s)
    }

    val draftHeaders = mutableListOf<SessionHeader>()
    for ((id, draftState) in draftEntries) {
        val serverHeader = filteredSessions.firstOrNull { it.id == id }
        if (serverHeader != null) {
            draftHeaders.add(serverHeader.copy(updated_at = draftState.draftUpdatedAt ?: serverHeader.updated_at))
        } else {
            val fallbackProj = projects.firstOrNull()
            draftHeaders.add(
                createEphemeralDraftHeader(
                    id = id,
                    draft = draftState,
                    projectId = fallbackProj?.id,
                    cwd = fallbackProj?.path,
                ),
            )
        }
    }

    val draftSection: GroupedProjectSection? = if (draftHeaders.isNotEmpty()) {
        draftHeaders.sortByDescending { it.updated_at }
        GroupedProjectSection(
            projectId = null,
            projectName = "Drafts",
            data = draftHeaders,
            latestAt = draftHeaders.first().updated_at,
        )
    } else null

    val draftIds = draftEntries.keys
    val seenIds = mutableSetOf<String>()
    val nonDraftSessions = filteredSessions.filter { s ->
        if (s.id in draftIds || s.id in seenIds) false
        else {
            seenIds.add(s.id)
            true
        }
    }

    data class ProjectGroup(val projectId: String?, val projectName: String, val list: MutableList<SessionHeader>)

    val byProject = linkedMapOf<String, ProjectGroup>()
    for (session in nonDraftSessions) {
        if (session.project_id == null) {
            val group = byProject.getOrPut("general") {
                ProjectGroup(null, "General", mutableListOf())
            }
            group.list.add(session)
            continue
        }

        val project = projects.firstOrNull { p ->
            (session.cwd.isNotEmpty() && (p.path == session.cwd || session.cwd.startsWith("${p.path}/"))) ||
                p.id == session.project_id
        }

        val resolvedId = project?.id ?: session.project_id
        val groupKey = "project-$resolvedId"
        val rawName = project?.name ?: folderName(session.cwd).ifBlank { "Drafts" }
        val groupName = formatProjectTitle(rawName)

        val group = byProject.getOrPut(groupKey) {
            ProjectGroup(resolvedId, groupName, mutableListOf())
        }
        group.list.add(session)
    }

    val sections = mutableListOf<GroupedProjectSection>()
    for ((_, group) in byProject) {
        val sorted = group.list.sortedByDescending { it.updated_at }
        sections.add(
            GroupedProjectSection(
                projectId = group.projectId,
                projectName = group.projectName,
                data = sorted,
                latestAt = sorted.firstOrNull()?.updated_at ?: 0L,
            ),
        )
    }

    val sortedSections = sections.sortedByDescending { it.latestAt }
    return if (draftSection != null) listOf(draftSection) + sortedSections else sortedSections
}

/**
 * The project a chat belongs to. The server's project link wins; the folder
 * is only a fallback for chats without one. Folder matching alone fails for a
 * worktree chat, whose cwd sits in the worktree directory, not under the project.
 */
fun projectForSession(
    projects: List<ProjectInfo>,
    projectId: String?,
    cwd: String?,
): ProjectInfo? {
    if (!projectId.isNullOrBlank()) projects.firstOrNull { it.id == projectId }?.let { return it }
    if (cwd.isNullOrEmpty()) return null
    return projects.firstOrNull { p -> p.path.isNotEmpty() && (p.path == cwd || cwd.startsWith(p.path + "/")) }
}

/**
 * Branch shown on a Home row. A worktree chat runs on its own branch, so that
 * wins; the project's branch is the main checkout's and is only right for chats
 * working in it.
 */
fun sessionBranch(worktreeBranch: String?, projectId: String?, projectBranches: Map<String, String>): String? {
    worktreeBranch?.takeIf { it.isNotBlank() }?.let { return it }
    return projectId?.let { projectBranches[it] }?.ifBlank { null }
}
