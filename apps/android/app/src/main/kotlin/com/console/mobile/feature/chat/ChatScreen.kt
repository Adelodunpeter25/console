package com.console.mobile.feature.chat

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.projectForSession
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.store.MobileTab
import com.console.mobile.ui.components.common.new.ChatLoadingSkeleton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.ScrollThumb
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.BrandGit
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.DeviceMobile
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * Port of screens/chat/chat-screen.tsx.
 * Header (title + files/changes/terminal shortcuts) → message list with run
 * activity + streaming footer → interaction panel or composer → todo sheet.
 */
@Composable
fun ChatScreen(
    onBackToHome: () -> Unit,
    onOpenTab: (MobileTab) -> Unit,
    onOpenSubagentDetails: (String) -> Unit,
    onAddProject: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val sessionId = appState.selectedSessionId
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val sessionStatuses by AppContainer.sessionStateHolder.statuses.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionChangesMap by AppContainer.sessionStateHolder.sessionChanges.collectAsStateWithLifecycle()
    val sessionChanges = sessionId?.let { sessionChangesMap[it] }.orEmpty()

    var inspectingChange by remember { mutableStateOf<SessionFileChange?>(null) }

    BackHandler {
        AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
        onBackToHome()
    }

    if (sessionId == null) {
        Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
            PageHeader(title = "Chat", onBack = { onBackToHome() })
            EmptyView(title = "No session selected", description = "Pick a chat from Home to get started.", icon = TablerIcons.Outline.Message, modifier = Modifier.fillMaxSize())
        }
        return
    }

    val chat: ChatSessionState = chatSessions[sessionId] ?: createChatSessionState()
    var loadingMessages by remember(sessionId) { mutableStateOf(chat.messages.isEmpty()) }
    var overflowMenu by remember { mutableStateOf(false) }
    var todoSheet by remember { mutableStateOf(false) }
    var subagentSheet by remember { mutableStateOf(false) }
    var subagentSelected by remember { mutableStateOf<String?>(null) }
    val listState = rememberLazyListState()

    // Load detail + hydrate todos/subagents/changes on entry; attach to server run if working.
    LaunchedEffect(sessionId) {
        loadingMessages = AppContainer.chatStateHolder.get(sessionId).messages.isEmpty()
        val header = coroutineScope {
            val detail = async { AppContainer.sessionRepository.loadDetail(sessionId) }
            launch { AppContainer.chatRepository.loadTodos(sessionId) }
            launch { AppContainer.chatRepository.loadSubagents(sessionId) }
            launch { AppContainer.sessionRepository.loadSessionChanges(sessionId) }
            detail.await()
        }
        loadingMessages = false
        if (header?.status == "working") {
            AppContainer.chatRepository.attachServerRun(sessionId)
        }
    }

    // Refresh file changes when an active run settles.
    LaunchedEffect(chat.running) {
        if (!chat.running) {
            AppContainer.sessionRepository.loadSessionChanges(sessionId)
        }
    }

    // A run can also start after entry — the desktop app, another device, or a
    // queued prompt — and the chat screen is where its output belongs.
    LaunchedEffect(sessionId) {
        AppContainer.sessionStateHolder.statuses
            .map { it[sessionId] }
            .distinctUntilChanged()
            .collect { status ->
                if (status == SessionStatus.Working) {
                    AppContainer.chatRepository.attachServerRun(sessionId)
                }
            }
    }

    val messages = chat.messages
    val displayMessages = visibleMessages(messages)
    val runs = remember(messages, chat.runs) {
        if (chat.runs.isNotEmpty()) chat.runs else reconstructRuns(messages)
    }
    val userRunMap = remember(displayMessages, runs) {
        val map = mutableMapOf<Int, Int>()
        var userCount = 0
        displayMessages.forEachIndexed { i, m ->
            if (m is com.console.mobile.data.model.UserMessage) {
                if (userCount < runs.size) map[i] = userCount
                userCount++
            }
        }
        map
    }
    val assistantTurnChanges = remember(displayMessages, sessionChanges) {
        buildAssistantTurnChangesMap(displayMessages, sessionChanges)
    }
    val latestUserIndex = remember(displayMessages) {
        displayMessages.indexOfLast { it is com.console.mobile.data.model.UserMessage }
    }
    val isStreaming = chat.running && (chat.streamingText.isNotEmpty() || chat.streamingThinking.isNotEmpty() || chat.activeToolCalls.isNotEmpty())
    val hasPending = chat.pendingPermissions.isNotEmpty() || chat.pendingQuestions.isNotEmpty()
    val hasMessages = messages.isNotEmpty()

    val todos = chat.todoItems
    val (todoDone, todoTotal) = todoCounts(todos)
    val hasActiveTodos = todos.any { it.status != "completed" && it.status != "done" && it.status != "complete" }
    val nextTodo = nextPendingTodo(todos)
    val subagents = chat.subagents
    val hasSubagents = subagents.isNotEmpty()

    val chatTitle = remember(sessionId, messages, projectState.sessions) {
        val header = projectState.sessions.firstOrNull { it.id == sessionId }
        header?.title?.ifBlank { "Chat" } ?: "Chat"
    }
    val keyboardController = LocalSoftwareKeyboardController.current
    val focusManager = androidx.compose.ui.platform.LocalFocusManager.current
    val view = sessionViews[sessionId]
    val cwd = view?.sessionCwd

    fun jumpToProjectTab(tab: MobileTab) {
        if (cwd != null) {
            val match = projectForSession(projectState.projects, view.projectId, cwd)
                ?: projectState.projects.firstOrNull { p -> p.path.endsWith(cwd) }
            if (match != null) AppContainer.appStateHolder.setSelectedProjectId(match.id)
        }
        AppContainer.appStateHolder.setActiveTab(tab)
        onOpenTab(tab)
    }

    val displaySize = displayMessages.size
    var following by remember(sessionId) { mutableStateOf(true) }
    LaunchedEffect(listState) {
        launch {
            listState.interactionSource.interactions.collect {
                if (it is DragInteraction.Start) {
                    following = false
                    keyboardController?.hide()
                    focusManager.clearFocus()
                }
            }
        }
        snapshotFlow { listState.isScrollInProgress }.collect { scrolling ->
            if (!scrolling) following = !listState.canScrollForward
        }
    }
    val showScrollBottom by remember { derivedStateOf { !following && listState.canScrollForward } }

    LaunchedEffect(listState) {
        snapshotFlow {
            val info = listState.layoutInfo
            val last = info.visibleItemsInfo.lastOrNull()
            Triple(info.totalItemsCount, last?.index, last?.size)
        }.collect {
            if (following && !listState.isScrollInProgress) {
                try { listState.scrollToBottom() } catch (_: Exception) {}
            }
        }
    }

    var settledInitialPage by remember(sessionId) { mutableStateOf(false) }
    LaunchedEffect(sessionId, displaySize) {
        if (settledInitialPage || displaySize <= 0) return@LaunchedEffect
        settledInitialPage = true
        following = true
        try { listState.scrollToBottom() } catch (_: Exception) {}
    }

    // --- Older-message pagination -------------------------------------------
    val atTop = listState.firstVisibleItemIndex <= 0
    var anchor by remember(sessionId) { mutableStateOf<ListAnchor?>(null) }
    var autoLoadBlocked by remember(sessionId) { mutableStateOf(false) }

    LaunchedEffect(atTop, chat.hasMoreMessages, chat.loadingOlder) {
        if (!atTop) {
            autoLoadBlocked = false
            return@LaunchedEffect
        }
        if (autoLoadBlocked || !chat.hasMoreMessages || chat.loadingOlder || displaySize <= 0) return@LaunchedEffect
        anchor = ListAnchor(
            index = listState.firstVisibleItemIndex,
            offset = listState.firstVisibleItemScrollOffset,
            sizeBefore = displaySize,
        )
        if (listState.canScrollForward) following = false
        if (!AppContainer.sessionRepository.loadOlder(sessionId)) autoLoadBlocked = true
    }

    LaunchedEffect(displaySize) {
        val a = anchor ?: return@LaunchedEffect
        val delta = displaySize - a.sizeBefore
        if (delta > 0) {
            try { listState.scrollToItem(a.index + delta, a.offset) } catch (_: Exception) {}
        }
        anchor = null
    }

    Box(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        when {
            loadingMessages && !hasMessages -> ChatLoadingSkeleton(modifier = Modifier.padding(top = PageHeaderHeight))
            !hasMessages && !isStreaming -> EmptyView(title = "Start the conversation", description = "Ask anything about your project.", icon = TablerIcons.Outline.Message, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight, bottom = 80.dp))
            else -> {
                LazyColumn(
                    state = listState,
                    contentPadding = PaddingValues(top = PageHeaderHeight + 8.dp, bottom = 120.dp),
                    modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
                ) {
                    itemsIndexed(displayMessages, key = { _, m -> m.id ?: "${m.createdAt}-$sessionId" }) { index, msg ->
                        MessageBubbleItem(
                            item = msg,
                            fileChanges = assistantTurnChanges[index].orEmpty(),
                            onOpenChange = { inspectingChange = it },
                        )
                        val runIdx = userRunMap[index]
                        if (runIdx != null && runIdx < runs.size) {
                            RunActivity(activity = runs[runIdx], running = chat.running && index == latestUserIndex, cwd = cwd)
                        }
                    }
                    if (isStreaming && (chat.streamingText.isNotEmpty() || chat.streamingThinking.isNotEmpty())) {
                        item(key = "streaming") {
                            AssistantBubble(textContent = chat.streamingText.ifBlank { null }, thinkingContent = chat.streamingThinking.ifBlank { null }, isStreaming = true, createdAt = null)
                        }
                    } else if (latestUserIndex == -1 && runs.isNotEmpty() && chat.running) {
                        item(key = "unattached-run") {
                            RunActivity(activity = runs.last(), running = true, cwd = cwd)
                        }
                    }
                }
                ScrollThumb(
                    state = listState,
                    modifier = Modifier.align(Alignment.CenterEnd).padding(end = 2.dp, top = PageHeaderHeight, bottom = 80.dp),
                )
            }
        }
        if (showScrollBottom) {
            FloatingActionButton(
                onClick = {
                    following = true
                    scope.launch { try { listState.scrollToBottom() } catch (_: Exception) {} }
                },
                modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 90.dp),
                containerColor = NewTheme.Raised,
                contentColor = NewTheme.TextPrimary,
            ) {
                Icon(TablerIcons.Outline.ChevronDown, contentDescription = "Scroll to bottom")
            }
        }

        PageHeader(
            title = chatTitle,
            blurred = true,
            modifier = Modifier.align(Alignment.TopCenter),
            onBack = {
                AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
                onBackToHome()
            },
            actions = {
                HeaderIconButton(TablerIcons.Outline.Folder, "Open file explorer", onClick = { jumpToProjectTab(MobileTab.Files) })
                Box(modifier = Modifier.padding(start = 4.dp)) {
                    HeaderIconButton(TablerIcons.Outline.DotsVertical, "More options", onClick = { overflowMenu = true })
                    OverflowMenu(expanded = overflowMenu, onDismissRequest = { overflowMenu = false }) {
                        OverflowMenuItem(
                            label = "Open diff",
                            icon = TablerIcons.Outline.BrandGit,
                            onClick = {
                                overflowMenu = false
                                jumpToProjectTab(MobileTab.Changes)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open devices",
                            icon = TablerIcons.Outline.DeviceMobile,
                            onClick = {
                                overflowMenu = false
                                jumpToProjectTab(MobileTab.Devices)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open terminal",
                            icon = TablerIcons.Outline.Terminal2,
                            onClick = {
                                overflowMenu = false
                                jumpToProjectTab(MobileTab.Terminal)
                            },
                        )
                    }
                }
            },
        )

        Box(modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth()) {
            if (hasPending) {
                Column {
                    if (hasActiveTodos) {
                        TodoBanner(completed = todoDone, total = todoTotal, nextTask = nextTodo?.content, onPress = { todoSheet = true })
                    }
                    if (hasSubagents) {
                        SubagentBanner(subagents = subagents, onPress = { subagentSheet = true })
                    }
                    InteractionPanel(sessionId = sessionId, permissions = chat.pendingPermissions, questions = chat.pendingQuestions)
                }
            } else {
                Column {
                    if (hasActiveTodos) {
                        TodoBanner(completed = todoDone, total = todoTotal, nextTask = nextTodo?.content, onPress = { todoSheet = true })
                    }
                    if (hasSubagents) {
                        SubagentBanner(subagents = subagents, onPress = { subagentSheet = true })
                    }
                    Composer(
                        sessionId = sessionId,
                        value = chat.input,
                        onChange = { AppContainer.chatRepository.setInput(sessionId, it) },
                        running = chat.running,
                        projectLocked = hasMessages,
                        onAddProject = onAddProject,
                        onSend = {
                            keyboardController?.hide()
                            AppContainer.chatRepository.sendMessage(sessionId)
                            following = true
                            scope.launch { try { listState.scrollToBottom() } catch (_: Exception) {} }
                        },
                        onStop = { AppContainer.chatRepository.abort(sessionId) },
                    )
                }
            }
        }
    }

    if (todoSheet) {
        TodoBottomSheet(items = todos, completed = todoDone, total = todoTotal, onDismiss = { todoSheet = false })
    }
    if (subagentSheet) {
        SubagentSheet(subagents = subagents, selectedId = subagentSelected, onSelect = { subagentSelected = it }, onDismiss = { subagentSheet = false }, onOpenDetails = { id ->
            subagentSheet = false
            AppContainer.appStateHolder.setSelectedSubagentId(id)
            AppContainer.appStateHolder.setActiveTab(MobileTab.SubagentDetails)
            onOpenSubagentDetails(id)
        })
    }

    ChatDiffSheet(
        change = inspectingChange,
        sessionId = sessionId,
        cwd = cwd,
        onDismiss = { inspectingChange = null },
    )

    @Suppress("UNUSED_EXPRESSION")
    LaunchedEffect(sessionStatuses[sessionId]) { }
}
