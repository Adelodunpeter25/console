package com.console.mobile.feature.home

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
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
import androidx.compose.material3.Icon
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
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
import com.console.mobile.ui.components.common.new.BlurredHeaderScrim
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.hideKeyboardOnScroll
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionHeader
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowLeft
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import io.github.lyxnx.compose.ui.tablericons.outline.X
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Port of screens/home/home-screen.tsx + components/home/session-list.tsx + hooks/useHomeSessions.
 * Grouped project sections (pinned, projects), search, pull-refresh, compose.
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
    val allAutomations by AppContainer.automationsRepository.automations.collectAsStateWithLifecycle()
    val allWebhooks by AppContainer.automationsRepository.webhooks.collectAsStateWithLifecycle()
    val automationSessionIds = remember(allAutomations) {
        allAutomations.filter { it.status == "active" || it.status == "paused" }.map { it.sessionId }.toSet()
    }
    val webhookSessionIds = remember(allWebhooks) {
        allWebhooks.filter { it.status == "active" }.map { it.sessionId }.toSet()
    }
    var searchOpen by remember { mutableStateOf(false) }
    var searchQuery by remember { mutableStateOf("") }
    BackHandler(enabled = searchOpen) {
        searchOpen = false
        searchQuery = ""
    }
    var refreshing by remember { mutableStateOf(false) }
    var activeSession by remember { mutableStateOf<SessionHeader?>(null) }
    var branches by remember { mutableStateOf<Map<String, String>>(emptyMap()) }
    // Section keys whose sessions are hidden (Pinned/Settled/project ids),
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
    LaunchedEffect(searchQuery) {
        val trimmed = searchQuery.trim()
        if (trimmed.isEmpty()) return@LaunchedEffect
        kotlinx.coroutines.delay(150)
        AppContainer.sessionRepository.searchSessions(trimmed)
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
        val proj = projectId?.let { id -> projectState.projects.firstOrNull { it.id == id } }
            ?: projectState.projects.firstOrNull()
        val id = AppContainer.sessionRepository.startNewChat(proj?.id, proj?.path)
        AppContainer.appStateHolder.openChatSession(id)
        onOpenChat(id)
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
                            val isSearchSection = section.projectId == "search"
                            val sectionKey = sectionCollapseKey(section)
                            val collapsed = !isSearchSection && sectionKey in collapsedSections
                            item(key = "header-${section.projectId ?: section.projectName}-$sIdx") {
                                SessionSectionHeader(
                                    section = section,
                                    isFirst = sIdx == 0,
                                    collapsed = collapsed,
                                    onToggle = { if (!isSearchSection) toggleSection(sectionKey) },
                                    onNewChat = if (!isSearchSection && section.projectId != null && section.projectId != "pinned" && section.projectId != "settled") {
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
                                                hasAutomation = session.id in automationSessionIds,
                                                hasWebhook = session.id in webhookSessionIds,
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
            AnimatedContent(
                targetState = searchOpen,
                transitionSpec = { fadeIn() togetherWith fadeOut() },
                modifier = Modifier.align(Alignment.TopCenter),
                label = "homeHeaderSearch",
            ) { searching ->
                if (searching) {
                    HomeSearchHeader(
                        query = searchQuery,
                        onQueryChange = { searchQuery = it },
                        onClose = {
                            searchOpen = false
                            searchQuery = ""
                        },
                    )
                } else {
                    PageHeader(
                        title = "Console",
                        blurred = true,
                        showSettings = false,
                        actions = {
                            HeaderIconButton(
                                icon = TablerIcons.Outline.Search,
                                contentDescription = "Search threads",
                                onClick = { searchOpen = true },
                                modifier = Modifier.padding(end = 4.dp),
                            )
                            EnvironmentSwitcher()
                        },
                    )
                }
            }
        }
    }

    val sheetSession = activeSession
    if (sheetSession != null) {
        SessionActionsSheet(session = sheetSession, onDismiss = { activeSession = null })
    }
}

@Composable
private fun HomeSearchHeader(
    query: String,
    onQueryChange: (String) -> Unit,
    onClose: () -> Unit,
) {
    val focusRequester = remember { FocusRequester() }
    LaunchedEffect(Unit) {
        try { focusRequester.requestFocus() } catch (_: Exception) {}
    }
    Box(modifier = Modifier.fillMaxWidth().height(PageHeaderHeight)) {
        BlurredHeaderScrim(backgroundColor = NewTheme.Background, blurRadius = 20.dp)
        Row(
            modifier = Modifier.fillMaxWidth().height(PageHeaderHeight).padding(horizontal = 16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            HeaderIconButton(
                icon = TablerIcons.Outline.ArrowLeft,
                contentDescription = "Close search",
                onClick = onClose,
                modifier = Modifier.padding(end = 8.dp),
            )
            BasicTextField(
                value = query,
                onValueChange = onQueryChange,
                singleLine = true,
                textStyle = TextStyle(color = NewTheme.TextPrimary, fontSize = 16.sp),
                cursorBrush = SolidColor(NewTheme.Accent),
                modifier = Modifier.weight(1f).focusRequester(focusRequester),
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (query.isEmpty()) {
                            Text("Search threads…", color = NewTheme.TextMuted, fontSize = 16.sp, maxLines = 1)
                        }
                        innerTextField()
                    }
                },
            )
            if (query.isNotEmpty()) {
                Box(
                    modifier = Modifier.size(28.dp).clip(CircleShape).clickable(onClickLabel = "Clear") { onQueryChange("") },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(TablerIcons.Outline.X, contentDescription = "Clear", tint = NewTheme.TextSecondary, modifier = Modifier.size(16.dp))
                }
            }
        }
    }
}

/** One scroll position for the session list; it is the only scrolling list on Home. */
private const val HomeSessionsScrollKey = "home:sessions"
