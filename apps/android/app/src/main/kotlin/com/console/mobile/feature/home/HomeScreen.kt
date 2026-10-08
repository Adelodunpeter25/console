package com.console.mobile.feature.home

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.GroupedProjectSection
import com.console.mobile.core.chat.buildGroupedProjectSections
import com.console.mobile.core.chat.draftPreview
import com.console.mobile.core.chat.formatProjectTitle
import com.console.mobile.core.chat.isDraftSession
import com.console.mobile.core.chat.sessionBranch
import com.console.mobile.core.util.folderName
import com.console.mobile.core.util.formatRelativeTime
import console.v1.SessionHeader
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.common.new.SearchBar
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.feature.home.EnvironmentSwitcher
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.SectionSkeleton
import com.console.mobile.ui.components.common.new.StatusPill
import com.console.mobile.ui.components.common.new.TintPill
import io.github.lyxnx.compose.ui.tablericons.outline.Search

/**
 * Port of screens/home/home-screen.tsx + components/home/session-list.tsx + hooks/useHomeSessions.
 * Grouped project sections (drafts first), search, pull-refresh, rename/delete sheet, compose.
 */
@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun HomeScreen(
    onOpenSettings: () -> Unit,
    onOpenChat: (String) -> Unit,
) {
    val scope = rememberCoroutineScope()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val sessionStatuses by AppContainer.sessionStateHolder.statuses.collectAsStateWithLifecycle()
    var searchQuery by remember { mutableStateOf("") }
    var refreshing by remember { mutableStateOf(false) }
    var creatingSession by remember { mutableStateOf(false) }
    var activeSession by remember { mutableStateOf<SessionHeader?>(null) }
    var branches by remember { mutableStateOf<Map<String, String>>(emptyMap()) }

    LaunchedEffect(Unit) {
        AppContainer.projectRepository.loadProjects()
        AppContainer.projectRepository.loadSessions()
    }
    LaunchedEffect(projectState.projects) {
        val projs = projectState.projects
        if (projs.isNotEmpty()) {
            branches = withContext(Dispatchers.IO) {
                try { AppContainer.gitRepository.fetchBranchesForProjects(projs) } catch (_: Exception) { emptyMap() }
            }
        }
    }

    val sections: List<GroupedProjectSection> = remember(projectState.sessions, projectState.projects, chatSessions, searchQuery) {
        buildGroupedProjectSections(projectState.sessions, projectState.projects, chatSessions, searchQuery)
    }
    val isLoading = projectState.sessionsLoading && sections.isEmpty()

    fun projectNameFor(s: SessionHeader): String {
        val proj = projectState.projects.firstOrNull { p ->
            (p.path.isNotEmpty() && s.cwd.isNotEmpty() && (p.path == s.cwd || s.cwd.startsWith(p.path + "/"))) || p.id == s.project_id
        }
        return proj?.name ?: folderName(s.cwd).ifBlank { "General" }.let { formatProjectTitle(it) }
    }
    fun branchFor(s: SessionHeader): String? {
        val proj = projectState.projects.firstOrNull { p ->
            (p.path.isNotEmpty() && s.cwd.isNotEmpty() && (p.path == s.cwd || s.cwd.startsWith(p.path + "/"))) || p.id == s.project_id
        }
        return sessionBranch(s.worktree?.branch, proj?.id ?: s.project_id, branches)
    }

    fun onRefresh() {
        refreshing = true
        scope.launch {
            try {
                withContext(Dispatchers.IO) {
                    AppContainer.projectRepository.loadProjects()
                    AppContainer.projectRepository.loadSessions()
                }
            } finally {
                refreshing = false
            }
        }
    }

    fun composeSession() {
        if (creatingSession) return
        creatingSession = true
        scope.launch {
            try {
                val project = projectState.projects.firstOrNull()
                val created = withContext(Dispatchers.IO) {
                    AppContainer.projectRepository.createSession(cwd = project?.path ?: "", projectId = project?.id, title = "New Chat")
                }
                AppContainer.appStateHolder.openChatSession(created.id)
                onOpenChat(created.id)
                // Navigate first, then load: loadDetail suspends until the fetch
                // lands, and holding the transition for that round trip showed a
                // visible stall on every new chat.
                launch { AppContainer.sessionRepository.loadDetail(created.id) }
            } catch (_: Exception) {
                confirmAlert("Unable to start chat", "Check the backend connection and try again.")
            } finally {
                creatingSession = false
            }
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(
            title = "Console",
            centerTitle = false,
            showSettings = true,
            onSettingsPress = onOpenSettings,
            actions = { EnvironmentSwitcher() },
        )
        PullToRefreshBox(
            isRefreshing = refreshing,
            onRefresh = ::onRefresh,
            modifier = Modifier.weight(1f).fillMaxWidth(),
        ) {
            if (isLoading) {
                Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    SectionSkeleton(rows = 3)
                    SectionSkeleton(rows = 2)
                }
            } else if (sections.isEmpty() && projectState.error != null) {
                LazyColumn(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    item {
                        EmptyView(
                            title = "Couldn't load chats",
                            description = projectState.error ?: "Failed to load chat sessions.",
                            icon = TablerIcons.Outline.AlertTriangle,
                            iconTint = NewTheme.Danger,
                        )
                    }
                }
            } else if (sections.isEmpty()) {
                LazyColumn(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    item {
                        EmptyView(
                            title = if (searchQuery.isNotBlank()) "No matching sessions" else "No chat sessions",
                            description = if (searchQuery.isNotBlank()) "No chats found matching \"$searchQuery\"." else "Start a new chat or select a project folder to get started.",
                            icon = if (searchQuery.isNotBlank()) TablerIcons.Outline.Search else TablerIcons.Outline.Message,
                        )
                    }
                }
            } else {
                val listState = rememberLazyListState()
                // A new query is a different list of rows, so the old offset
                // lands somewhere arbitrary (or out of bounds once results
                // shrink). Jump to the top so the first match is visible as
                // the user types. Instant rather than animated — the content
                // is swapped out, so a scroll animation would be a lie.
                LaunchedEffect(searchQuery) { listState.scrollToItem(0) }
                LazyColumn(state = listState, modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    sections.forEachIndexed { sIdx, section ->
                        item(key = "header-${section.projectId ?: section.projectName}-$sIdx") {
                            Row(
                                modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp, top = 14.dp, bottom = 4.dp),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                // The accent heading every grouped screen uses, with the folder glyph.
                                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.weight(1f)) {
                                    Icon(TablerIcons.Outline.FolderOpen, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(17.dp))
                                    Text(section.projectName, color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 8.dp))
                                }
                                if (section.projectId != null && section.projectName != "Drafts") {
                                    IconButton(
                                        onClick = {
                                            scope.launch {
                                                try {
                                                    val created = withContext(Dispatchers.IO) {
                                                        val proj = projectState.projects.firstOrNull { it.id == section.projectId }
                                                        AppContainer.projectRepository.createSession(cwd = proj?.path ?: "", projectId = section.projectId, title = "New Chat")
                                                    }
                                                    AppContainer.appStateHolder.openChatSession(created.id)
                                                    onOpenChat(created.id)
                                                    launch { AppContainer.sessionRepository.loadDetail(created.id) }
                                                } catch (_: Exception) {
                                                    confirmAlert("Unable to start chat", "Check the backend connection and try again.")
                                                }
                                            }
                                        },
                                        modifier = Modifier.size(36.dp),
                                    ) {
                                        Icon(TablerIcons.Outline.Plus, contentDescription = "New chat in ${section.projectName}", tint = NewTheme.TextSecondary, modifier = Modifier.size(20.dp))
                                    }
                                }
                            }
                        }
                        item(key = "card-${section.projectId ?: section.projectName}-$sIdx") {
                            SectionCard(modifier = Modifier.padding(bottom = 10.dp)) {
                                section.data.forEachIndexed { index, session ->
                                    val draft = chatSessions[session.id]
                                    val isDraft = draft != null && isDraftSession(draft)
                                    val preview = if (isDraft) draftPreview(draft) else null
                                    val status: SessionStatus? = sessionStatuses[session.id]
                                        ?: SessionStatus.fromValue(session.status)
                                    SessionRow(
                                        session = session,
                                        projectName = projectNameFor(session),
                                        branch = branchFor(session),
                                        status = status,
                                        isDraft = isDraft,
                                        draftPreview = preview,
                                        onClick = {
                                            scope.launch { AppContainer.sessionRepository.loadDetail(session.id) }
                                            AppContainer.appStateHolder.openChatSession(session.id)
                                            onOpenChat(session.id)
                                        },
                                        onLongPress = { activeSession = session },
                                    )
                                    if (index < section.data.lastIndex) SectionDivider()
                                }
                            }
                        }
                    }
                }
            }
        }
        SearchBar(
            value = searchQuery,
            onValueChange = { searchQuery = it },
            onComposePress = ::composeSession,
            composeEnabled = !creatingSession,
        )
    }

    val sheetSession = activeSession
    if (sheetSession != null) {
        var renameOpen by remember(sheetSession.id) { mutableStateOf(false) }
        BaseSheet(onDismiss = { activeSession = null }) {
            SectionCard {
                ActionRow(icon = TablerIcons.Outline.Edit, title = "Rename") { renameOpen = true }
                SectionDivider()
                ActionRow(icon = TablerIcons.Outline.Trash, title = "Delete", tint = NewTheme.Danger) {
                    activeSession = null
                    confirmAlert(
                        "Delete Chat",
                        "Are you sure you want to delete \"${sheetSession.title.ifBlank { "Untitled Session" }}\"?",
                        listOf(
                            ConfirmButton("Cancel", cancel = true),
                            ConfirmButton("Delete", destructive = true, onPress = {
                                scope.launch {
                                    try {
                                        withContext(Dispatchers.IO) { AppContainer.projectRepository.deleteSession(sheetSession.id) }
                                    } catch (e: Exception) {
                                        confirmAlert("Failed", e.message ?: "Unable to delete chat.")
                                    }
                                }
                            }),
                        ),
                    )
                }
            }
        }
        if (renameOpen) {
            var renameValue by remember(sheetSession.id) { mutableStateOf(sheetSession.title) }
            // A sheet inside the sheet: renaming is a one-field form, so it gets the
            // same field + primary button as every other form instead of a system dialog.
            BaseSheet(onDismiss = { renameOpen = false }, title = "Rename chat") {
                TextInput("Title", renameValue, { renameValue = it }, "Chat title")
                ActionButton(
                    text = "Save",
                    onClick = {
                        renameOpen = false
                        activeSession = null
                        scope.launch {
                            try {
                                withContext(Dispatchers.IO) {
                                    AppContainer.projectRepository.updateSession(sheetSession.id, UpdateSessionDto(title = renameValue.trim().ifBlank { sheetSession.title }))
                                }
                            } catch (e: Exception) {
                                confirmAlert("Failed", e.message ?: "Unable to rename chat.")
                            }
                        }
                    },
                    kind = ActionButtonKind.Primary,
                    enabled = renameValue.isNotBlank(),
                    modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
                )
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun SessionRow(
    session: SessionHeader,
    projectName: String,
    branch: String?,
    status: SessionStatus?,
    isDraft: Boolean,
    draftPreview: String?,
    onClick: () -> Unit,
    onLongPress: () -> Unit,
) {
    // Top-aligned, with a fixed first line shared by the title and the status pill.
    // Centring the right column on the whole row put "Ready" lower than the "Draft"
    // pill beside the title; giving both the same line height puts their centres level.
    Row(
        modifier = Modifier.fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongPress)
            .padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
            Row(modifier = Modifier.height(FirstLineHeight), verticalAlignment = Alignment.CenterVertically) {
                Text(
                    session.title.ifBlank { "Untitled Session" },
                    color = NewTheme.TextPrimary,
                    fontSize = 17.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f, fill = false),
                )
                if (isDraft) TintPill("Draft", NewTheme.Warning, Modifier.padding(start = 8.dp))
            }
            if (isDraft && draftPreview != null) {
                Text(draftPreview, color = NewTheme.Warning, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
            }
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 1.dp)) {
                Text(projectName, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                if (branch != null) {
                    Text(" • ", color = NewTheme.TextMuted, fontSize = 13.sp)
                    Text(branch, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                }
            }
        }
        Column(horizontalAlignment = Alignment.End) {
            Box(modifier = Modifier.height(FirstLineHeight), contentAlignment = Alignment.CenterEnd) { StatusPill(status) }
            val ts = if (session.updated_at > 0) session.updated_at else session.created_at
            Text(shortRelative(ts), color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 1.dp))
        }
    }
}

/** Height of a row's first line: the title and the status pill are both centred in it. */
private val FirstLineHeight = 28.dp

private fun shortRelative(ts: Long): String {
    if (ts <= 0) return ""
    return formatRelativeTime(ts).replace(" ago", "").replace("just now", "now")
}
