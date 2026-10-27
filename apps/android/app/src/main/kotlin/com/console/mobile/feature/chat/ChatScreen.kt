package com.console.mobile.feature.chat

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
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
import com.console.mobile.core.chat.runningBackgroundShells
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.store.MobileTab
import com.console.mobile.feature.subagents.SubagentDetailsSheet
import com.console.mobile.ui.components.common.new.ChatLoadingSkeleton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.ScrollThumb
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.isActive
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
    onAddProject: () -> Unit,
    onOpenPorts: () -> Unit,
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
    var todoSheet by remember { mutableStateOf(false) }
    var subagentPanelOpen by remember(sessionId) { mutableStateOf(false) }
    var selectedSubagentId by remember(sessionId) { mutableStateOf<String?>(null) }
    var shellSheet by remember { mutableStateOf(false) }

    // Load detail + hydrate todos/subagents/changes on entry; attach to server run if working.
    LaunchedEffect(sessionId) {
        loadingMessages = AppContainer.chatStateHolder.get(sessionId).messages.isEmpty()
        val header = coroutineScope {
            val detail = async { AppContainer.sessionRepository.loadDetail(sessionId) }
            launch { AppContainer.chatRepository.loadTodos(sessionId) }
            launch { AppContainer.chatRepository.loadSubagents(sessionId) }
            launch { AppContainer.chatRepository.loadBackgroundWork(sessionId) }
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
    val subagents = chat.subagents
    val liveShells = runningBackgroundShells(chat.backgroundWork)
    val hasLiveShells = liveShells.isNotEmpty()

    // A detached shell outlives its run, so no SSE stream is attached while it
    // keeps running. Poll only while shells are live to keep the chip honest.
    LaunchedEffect(sessionId, hasLiveShells) {
        if (!hasLiveShells) return@LaunchedEffect
        while (isActive) {
            delay(5_000)
            AppContainer.chatRepository.loadBackgroundWork(sessionId)
        }
    }

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
    val transcriptScroll = rememberChatTranscriptScroll(
        sessionId = sessionId,
        messagesLoaded = chat.messages.isNotEmpty(),
        displaySize = displaySize,
        hasMoreMessages = chat.hasMoreMessages,
        loadingOlder = chat.loadingOlder,
        loadOlder = { AppContainer.sessionRepository.loadOlder(sessionId) },
        onDragStarted = {
            keyboardController?.hide()
            focusManager.clearFocus()
        },
    )

    Box(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        when {
            loadingMessages && !hasMessages -> ChatLoadingSkeleton(modifier = Modifier.padding(top = PageHeaderHeight))
            !hasMessages && !isStreaming -> EmptyView(title = "Start the conversation", description = "Ask anything about your project.", icon = TablerIcons.Outline.Message, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight, bottom = 80.dp))
            else -> {
                ChatTranscript(
                    sessionId = sessionId,
                    messages = displayMessages,
                    listState = transcriptScroll.listState,
                    runs = runs,
                    userRunMap = userRunMap,
                    assistantTurnChanges = assistantTurnChanges,
                    latestUserIndex = latestUserIndex,
                    running = chat.running,
                    isStreaming = isStreaming,
                    streamingText = chat.streamingText,
                    streamingThinking = chat.streamingThinking,
                    cwd = cwd,
                    onOpenChange = { inspectingChange = it },
                    onFork = { messageId ->
                        scope.launch {
                            try {
                                val forked = AppContainer.projectRepository.forkSession(sessionId, messageId)
                                AppContainer.appStateHolder.setSelectedSessionId(forked.id)
                            } catch (_: Exception) {
                            }
                        }
                    },
                )
                ScrollThumb(
                    state = transcriptScroll.listState,
                    modifier = Modifier.align(Alignment.CenterEnd).padding(end = 2.dp, top = PageHeaderHeight, bottom = 80.dp),
                )
            }
        }

        ChatHeader(
            title = chatTitle,
            onBack = {
                AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
                onBackToHome()
            },
            onOpenFiles = { jumpToProjectTab(MobileTab.Files) },
            onOpenChanges = { jumpToProjectTab(MobileTab.Changes) },
            onOpenPorts = onOpenPorts,
            onOpenDevices = { jumpToProjectTab(MobileTab.Devices) },
            onOpenTerminal = { jumpToProjectTab(MobileTab.Terminal) },
            modifier = Modifier.align(Alignment.TopCenter),
        )

        Box(modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth()) {
            Column {
                // Above the pills so it can never sit on them; it only shows
                // while the reader is scrolled away from the latest message.
                if (transcriptScroll.showScrollToBottom) {
                    Box(
                        modifier = Modifier.fillMaxWidth().padding(end = 16.dp).padding(bottom = 8.dp),
                        contentAlignment = Alignment.CenterEnd,
                    ) {
                        FloatingActionButton(
                            onClick = {
                                transcriptScroll.scrollToBottom(scope)
                            },
                            containerColor = NewTheme.Raised,
                            contentColor = NewTheme.TextPrimary,
                        ) {
                            Icon(TablerIcons.Outline.ChevronDown, contentDescription = "Scroll to bottom")
                        }
                    }
                }
                if (subagentPanelOpen && subagents.isNotEmpty()) {
                    SubagentFloatingPanel(subagents = subagents, onOpenDetails = { id ->
                        subagentPanelOpen = false
                        selectedSubagentId = id
                    })
                }
                ComposerPills(
                    completedTasks = todoDone,
                    totalTasks = todoTotal,
                    showTasks = hasActiveTodos,
                    subagents = subagents,
                    shellCount = liveShells.size,
                    onTasks = { todoSheet = true },
                    onSubagents = { subagentPanelOpen = !subagentPanelOpen },
                    onShells = { shellSheet = true },
                )
                if (hasPending) {
                    InteractionPanel(sessionId = sessionId, permissions = chat.pendingPermissions, questions = chat.pendingQuestions)
                } else {
                    Composer(
                        sessionId = sessionId,
                        value = chat.input,
                        onChange = { AppContainer.chatRepository.setInput(sessionId, it) },
                        running = chat.running,
                        projectLocked = hasMessages,
                        onAddProject = onAddProject,
                        onSend = {
                            subagentPanelOpen = false
                            keyboardController?.hide()
                            AppContainer.chatRepository.sendMessage(sessionId)
                            transcriptScroll.scrollToBottom(scope)
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
    val sheetSubagent = subagents.firstOrNull { it.subagent_id == selectedSubagentId }
    if (sheetSubagent != null) {
        SubagentDetailsSheet(subagent = sheetSubagent, onDismiss = { selectedSubagentId = null })
    }
    if (shellSheet) {
        BackgroundShellSheet(
            sessionId = sessionId,
            works = liveShells,
            onCancel = { workId -> AppContainer.chatRepository.cancelBackgroundWork(sessionId, workId) },
            onDismiss = { shellSheet = false },
        )
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
