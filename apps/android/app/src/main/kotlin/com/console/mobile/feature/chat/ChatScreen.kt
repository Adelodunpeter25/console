package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowDownward
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.GitHub
import androidx.compose.material.icons.filled.Message
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.IconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.store.MobileTab
import com.console.mobile.ui.components.ChatScreenSkeleton
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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
) {
    val scope = rememberCoroutineScope()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val sessionId = appState.selectedSessionId
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val sessionStatuses by AppContainer.sessionStateHolder.statuses.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()

    if (sessionId == null) {
        Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
            ScreenHeader(title = "Chat", onBack = { onBackToHome() })
            EmptyState(title = "No session selected", description = "Pick a chat from Home to get started.", icon = { Icon(Icons.Filled.Message, contentDescription = null, tint = ConsoleColors.TextMuted) })
        }
        return
    }

    val chat: ChatSessionState = chatSessions[sessionId] ?: com.console.mobile.core.chat.createChatSessionState()
    var loadingMessages by remember(sessionId) { mutableStateOf(chat.messages.isEmpty()) }
    var todoSheet by remember { mutableStateOf(false) }
    var subagentSheet by remember { mutableStateOf(false) }
    var subagentSelected by remember { mutableStateOf<String?>(null) }
    val listState = rememberLazyListState()

    // Load detail + hydrate todos/subagents on entry; attach to server run if working.
    LaunchedEffect(sessionId) {
        loadingMessages = AppContainer.chatStateHolder.get(sessionId).messages.isEmpty()
        withContext(Dispatchers.IO) {
            try { AppContainer.sessionRepository.loadDetail(sessionId) } catch (_: Exception) {}
        }
        loadingMessages = false
        AppContainer.chatRepository.loadTodos(sessionId)
        AppContainer.chatRepository.loadSubagents(sessionId)
        val serverStatus = AppContainer.sessionStateHolder.statuses.value[sessionId]
        if (serverStatus == SessionStatus.Working) {
            AppContainer.chatRepository.attachServerRun(sessionId)
        }
    }

    val messages = chat.messages
    val displayMessages = visibleMessages(messages)
    val runs = remember(messages) {
        if (chat.runs.isNotEmpty()) chat.runs else reconstructRuns(messages)
    }
    // Map user-message index → run (tool turns are filtered from display list).
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

    val chatTitle = remember(sessionId, messages) {
        val header = AppContainer.projectStateHolder.state.value.sessions.firstOrNull { it.id == sessionId }
        header?.title?.ifBlank { "Chat" } ?: "Chat"
    }
    val view = sessionViews[sessionId]
    val cwd = view?.sessionCwd

    fun jumpToProjectTab(tab: MobileTab) {
        if (cwd != null) {
            val match = projectState.projects.firstOrNull { p -> p.path == cwd || cwd.startsWith(p.path + "/") || p.path.endsWith(cwd) }
            if (match != null) AppContainer.appStateHolder.setSelectedProjectId(match.id)
        }
        AppContainer.appStateHolder.setActiveTab(tab)
        onOpenTab(tab)
    }

    // Auto-follow while streaming.
    LaunchedEffect(chat.streamingText.length, chat.streamingThinking.length, messages.size, chat.activeToolCalls.size) {
        if (chat.running || isStreaming) {
            try { listState.animateScrollToItem(maxOf(0, displayMessages.size - 1)) } catch (_: Exception) {}
        }
    }
    val showScrollBottom by remember {
        derivedStateOf {
            val last = listState.layoutInfo.visibleItemsInfo.lastOrNull()
            last != null && last.index < displayMessages.size - 1 && displayMessages.size > 2
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = chatTitle,
            onBack = {
                AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
                onBackToHome()
            },
            actions = {
                IconButton(onClick = { jumpToProjectTab(MobileTab.Files) }, modifier = Modifier.size(40.dp)) {
                    Icon(Icons.Filled.Folder, contentDescription = "Open file explorer", tint = Color.White)
                }
                IconButton(onClick = { jumpToProjectTab(MobileTab.Changes) }, modifier = Modifier.size(40.dp)) {
                    Icon(Icons.Filled.GitHub, contentDescription = "Open changes", tint = Color.White)
                }
                IconButton(onClick = { jumpToProjectTab(MobileTab.Terminal) }, modifier = Modifier.size(40.dp)) {
                    Icon(Icons.Filled.Terminal, contentDescription = "Open terminal", tint = Color.White)
                }
            },
        )
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                loadingMessages && !hasMessages -> ChatScreenSkeleton()
                !hasMessages && !isStreaming -> EmptyState(title = "Start the conversation", description = "Ask anything about your project.", icon = { Icon(Icons.Filled.Message, contentDescription = null, tint = ConsoleColors.TextMuted) })
                else -> LazyColumn(state = listState, modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                    itemsIndexed(displayMessages, key = { _, m -> m.id ?: "${m.createdAt}-$sessionId" }) { index, msg ->
                        MessageBubbleItem(item = msg)
                        val runIdx = userRunMap[index]
                        if (runIdx != null && runIdx < runs.size) {
                            RunActivity(activity = runs[runIdx], running = chat.running && index == latestUserIndex, cwd = cwd)
                        }
                    }
                    // Streaming footer: unattached latest run + live bubble.
                    if (isStreaming && (chat.streamingText.isNotEmpty() || chat.streamingThinking.isNotEmpty())) {
                        item(key = "streaming") {
                            AssistantBubble(textContent = chat.streamingText.ifBlank { null }, thinkingContent = chat.streamingThinking.ifBlank { null }, isStreaming = true, createdAt = null)
                        }
                    } else if (latestUserIndex == -1 && runs.isNotEmpty() && chat.running) {
                        item(key = "unattached-run") {
                            RunActivity(activity = runs.last(), running = true, cwd = cwd)
                        }
                    }
                    // Live tool calls not yet in runs.
                    if (chat.activeToolCalls.isNotEmpty()) {
                        items(chat.activeToolCalls.size) { i ->
                            val call = chat.activeToolCalls[i]
                            ToolActivityRow(name = call.name, isRunning = true, isError = false, detail = "Running")
                        }
                    }
                }
            }
            if (showScrollBottom) {
                androidx.compose.material3.FloatingActionButton(
                    onClick = { scope.launch { try { listState.animateScrollToItem(maxOf(0, displayMessages.size - 1)) } catch (_: Exception) {} } },
                    modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 16.dp),
                    containerColor = ConsoleColors.SurfaceElevated,
                    contentColor = ConsoleColors.TextPrimary,
                ) {
                    Icon(Icons.Filled.ArrowDownward, contentDescription = "Scroll to bottom")
                }
            }
        }
        if (hasPending) {
            if (hasActiveTodos) {
                TodoBanner(completed = todoDone, total = todoTotal, nextTask = nextTodo?.content, onPress = { todoSheet = true })
            }
            if (hasSubagents) {
                SubagentBanner(subagents = subagents, onPress = { subagentSheet = true })
            }
            InteractionPanel(sessionId = sessionId, permissions = chat.pendingPermissions, questions = chat.pendingQuestions)
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
                    onSend = {
                        AppContainer.chatRepository.sendMessage(sessionId)
                        scope.launch { try { listState.animateScrollToItem(maxOf(0, displayMessages.size)) } catch (_: Exception) {} }
                    },
                    onStop = { AppContainer.chatRepository.abort(sessionId) },
                )
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

    // Consume streaming errors surfaced as messages — scroll already follows.
    @Suppress(\"UNUSED_EXPRESSION\")\n    LaunchedEffect(sessionStatuses[sessionId]) { }\n}
