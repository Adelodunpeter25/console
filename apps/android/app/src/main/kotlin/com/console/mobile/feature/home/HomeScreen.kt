package com.console.mobile.feature.home

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Text
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.GroupedProjectSection
import com.console.mobile.core.chat.buildGroupedProjectSections
import com.console.mobile.core.chat.draftPreview
import com.console.mobile.core.chat.formatProjectTitle
import com.console.mobile.core.chat.isDraftSession
import com.console.mobile.core.chat.sessionBranch
import com.console.mobile.core.util.ScrollMemory
import com.console.mobile.core.util.folderName
import com.console.mobile.feature.home.EnvironmentSwitcher
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.components.common.new.BlurredBottomBar
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.SearchBar
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.hideKeyboardOnScroll
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionHeader
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Port of screens/home/home-screen.tsx + components/home/session-list.tsx + hooks/useHomeSessions.
 * Grouped project sections (pinned, drafts, projects), search, pull-refresh, compose.
 *
 * The row, section header, and long-press sheet live in [SessionRow],
 * [SessionSectionHeader], and [SessionActionsSheet].
 */
@OptIn(ExperimentalMaterial3Api::class)
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
    // Section keys whose sessions are hidden (Pinned/Drafts/Settled/project ids),
    // persisted across app restarts.
    var collapsedSections by remember { mutableStateOf(AppContainer.preferencesStore.loadCollapsedSections()) }
    val toggleSection: (String) -> Unit = { key ->
        collapsedSections = if (key in collapsedSections) collapsedSections - key else collapsedSections + key
        AppContainer.preferencesStore.saveCollapsedSections(collapsedSections)
    }

    LaunchedEffect(Unit) {
        AppContainer.projectRepository.loadProjects()
        AppContainer.sessionRepository.loadSessions()
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
                    AppContainer.sessionRepository.loadSessions()
                }
            } finally {
                refreshing = false
            }
        }
    }

    fun startChat(projectId: String?) {
        if (creatingSession) return
        creatingSession = true
        scope.launch {
            try {
                val proj = projectId?.let { id -> projectState.projects.firstOrNull { it.id == id } }
                    ?: projectState.projects.firstOrNull()
                val created = withContext(Dispatchers.IO) {
                    AppContainer.sessionRepository.createSession(cwd = proj?.path ?: "", projectId = proj?.id, title = "New Chat")
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
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            PullToRefreshBox(
                isRefreshing = refreshing,
                onRefresh = ::onRefresh,
                modifier = Modifier.fillMaxSize(),
            ) {
                if (isLoading) {
                    Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp).padding(top = PageHeaderHeight)) {
                        HomeSkeleton()
                    }
                } else if (sections.isEmpty() && projectState.error != null) {
                    LazyColumn(
                        contentPadding = PaddingValues(top = PageHeaderHeight + 8.dp, bottom = 96.dp),
                        modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    ) {
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
                    LazyColumn(
                        contentPadding = PaddingValues(top = PageHeaderHeight + 8.dp, bottom = 96.dp),
                        modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    ) {
                        item {
                            EmptyView(
                                title = if (searchQuery.isNotBlank()) "No matching sessions" else "No chat sessions",
                                description = if (searchQuery.isNotBlank()) "No chats found matching \"$searchQuery\"." else "Start a new chat or select a project folder to get started.",
                                icon = if (searchQuery.isNotBlank()) TablerIcons.Outline.Search else TablerIcons.Outline.Message,
                            )
                        }
                    }
                } else {
                    val restored = remember { ScrollMemory.get(HomeSessionsScrollKey) }
                    val listState = rememberLazyListState(
                        initialFirstVisibleItemIndex = restored?.index ?: 0,
                        initialFirstVisibleItemScrollOffset = restored?.offset ?: 0,
                    )
                    // Home is rebuilt whenever the chat screen is closed, so
                    // keep its place continuously rather than on dispose.
                    LaunchedEffect(listState) {
                        snapshotFlow {
                            listState.firstVisibleItemIndex to listState.firstVisibleItemScrollOffset
                        }.collect { (index, offset) ->
                            ScrollMemory.save(HomeSessionsScrollKey, index, offset)
                        }
                    }
                    // A new query is a different list of rows, so the old offset
                    // lands somewhere arbitrary (or out of bounds once results
                    // shrink). Jump to the top as the user types — but not on
                    // entry, where the remembered position is being restored.
                    LaunchedEffect(listState) {
                        snapshotFlow { searchQuery }
                            .drop(1)
                            .collect { listState.scrollToItem(0) }
                    }
                    LazyColumn(
                        state = listState,
                        contentPadding = PaddingValues(top = PageHeaderHeight, bottom = 96.dp),
                        modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp).hideKeyboardOnScroll(),
                    ) {
                        sections.forEachIndexed { sIdx, section ->
                            val sectionKey = sectionCollapseKey(section)
                            val collapsed = sectionKey in collapsedSections
                            item(key = "header-${section.projectId ?: section.projectName}-$sIdx") {
                                SessionSectionHeader(
                                    section = section,
                                    isFirst = sIdx == 0,
                                    collapsed = collapsed,
                                    onToggle = { toggleSection(sectionKey) },
                                    onNewChat = if (section.projectId != null && section.projectId != "pinned" && section.projectId != "settled" && section.projectName != "Drafts") {
                                        { startChat(section.projectId) }
                                    } else {
                                        null
                                    },
                                )
                            }
                            if (!collapsed) {
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
            }
            PageHeader(
                title = "Console",
                blurred = true,
                showSettings = true,
                onSettingsPress = onOpenSettings,
                actions = { EnvironmentSwitcher() },
                modifier = Modifier.align(Alignment.TopCenter),
            )
            // The search bar rides over the list rather than sitting below it:
            // the rows scroll underneath it and the frosted bar fades them out
            // instead of being cut off at a hard edge.
            BlurredBottomBar(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)),
            ) {
                SearchBar(
                    value = searchQuery,
                    onValueChange = { searchQuery = it },
                    onComposePress = { startChat(projectState.projects.firstOrNull()?.id) },
                    composeEnabled = !creatingSession,
                )
            }
        }
    }

    val sheetSession = activeSession
    if (sheetSession != null) {
        SessionActionsSheet(session = sheetSession, onDismiss = { activeSession = null })
    }
}

/** One scroll position for the session list; it is the only scrolling list on Home. */
private const val HomeSessionsScrollKey = "home:sessions"
