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
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.BrandGit
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.Files
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.ChatSessionState
import com.console.mobile.core.chat.createChatSessionState
import com.console.mobile.core.chat.reconstructRuns
import com.console.mobile.data.model.SessionStatus
import com.console.mobile.data.store.MobileTab
import com.console.mobile.ui.components.ChatScreenSkeleton
import com.console.mobile.ui.components.EdgeScrollIndicator
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.launch

/** Scroll position to restore after older messages are prepended. */
private data class ListAnchor(
    val index: Int,
    val offset: Int,
    val sizeBefore: Int,
)

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
            EmptyState(title = "No session selected", description = "Pick a chat from Home to get started.", icon = { Icon(TablerIcons.Outline.Message, contentDescription = null, tint = ConsoleColors.TextMuted) })
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
    // The three fetches are independent, so they run concurrently instead of
    // as three sequential round trips — that latency was most of the wait on
    // opening a session.
    LaunchedEffect(sessionId) {
        loadingMessages = AppContainer.chatStateHolder.get(sessionId).messages.isEmpty()
        coroutineScope {
            launch { AppContainer.sessionRepository.loadDetail(sessionId) }
            launch { AppContainer.chatRepository.loadTodos(sessionId) }
            launch { AppContainer.chatRepository.loadSubagents(sessionId) }
        }
        loadingMessages = false
        val serverStatus = AppContainer.sessionStateHolder.statuses.value[sessionId]
        if (serverStatus == SessionStatus.Working) {
            AppContainer.chatRepository.attachServerRun(sessionId)
        }
    }

    val messages = chat.messages
    val displayMessages = visibleMessages(messages)
    // Key on chat.runs too: turnEnd/turnStart/toolExecutionEnd mutate runs without
    // touching messages, and those mutations drive status + elapsed time.
    val runs = remember(messages, chat.runs) {
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
    val keyboardController = LocalSoftwareKeyboardController.current
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

    // Only the scroll position belongs in derivedStateOf — it's the one thing here
    // that is snapshot-tracked. displayMessages is a plain local recomputed each
    // composition, so reading it inside a remembered derived state froze the
    // thresholds at first composition and hid the button for the whole session.
    val lastVisibleIndex by remember {
        derivedStateOf { listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1 }
    }
    val displaySize = displayMessages.size
    val showScrollBottom = lastVisibleIndex >= 0 && lastVisibleIndex < displaySize - 1 && displaySize > 2

    // Auto-follow while streaming. Keyed on the latest message's id rather than
    // the message count: a pagination prepend changes the count without adding
    // anything new at the bottom, and must not yank the user to the end. Gated on
    // !showScrollBottom so it stops fighting the user the moment they scroll up
    // (e.g. to reread earlier tool-call output) instead of yanking them back down
    // on the next streamed chunk.
    val latestMessageId = messages.lastOrNull()?.id
    LaunchedEffect(chat.streamingText.length, chat.streamingThinking.length, latestMessageId, chat.activeToolCalls.size) {
        if ((chat.running || isStreaming) && !showScrollBottom) {
            try { listState.animateScrollToItem(maxOf(0, displayMessages.size - 1)) } catch (_: Exception) {}
        }
    }

    // Opening a session lands on the newest page, so the list has to be jumped
    // to the bottom once that page arrives. The streaming auto-follow above
    // can't do it: it's gated on an active run, and this case is an idle chat.
    // Keyed on the message count so it fires exactly once per entry — a later
    // prepend or append must not drag the user back down over their scroll.
    var settledInitialPage by remember(sessionId) { mutableStateOf(false) }
    LaunchedEffect(sessionId, displaySize) {
        if (settledInitialPage || displaySize <= 0) return@LaunchedEffect
        settledInitialPage = true
        try { listState.scrollToItem(displaySize - 1) } catch (_: Exception) {}
    }

    // --- Older-message pagination -------------------------------------------
    // Opening a session only fetches the newest page, so the user scrolls to
    // the top to walk backwards through history. Derived as a Boolean: a
    // LaunchedEffect key needs a stable type, and a raw index would relaunch
    // on every single index change.
    val atTop = listState.firstVisibleItemIndex <= 0

    // Prepended rows push everything down, so the view would visibly lurch
    // when the page lands. Snapshot which row the user was looking at plus the
    // list size at that moment; once the size grows, shift by exactly the
    // number of new rows and the same message stays under their thumb.
    var anchor by remember(sessionId) { mutableStateOf<ListAnchor?>(null) }
    // Set when a page fails, so the effect below doesn't spin retrying against a
    // failing backend while the user sits at the top. Deliberately not a key:
    // writing it must not relaunch this effect. Leaving the top re-arms it.
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

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = chatTitle,
            onBack = {
                AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
                onBackToHome()
            },
            actions = {
                IconButton(onClick = { jumpToProjectTab(MobileTab.Files) }, modifier = Modifier.size(40.dp)) {
                    Icon(TablerIcons.Outline.Files, contentDescription = "Open file explorer", tint = Color.White)
                }
                IconButton(onClick = { jumpToProjectTab(MobileTab.Changes) }, modifier = Modifier.size(40.dp)) {
                    Icon(TablerIcons.Outline.BrandGit, contentDescription = "Open changes", tint = Color.White)
                }
                IconButton(onClick = { jumpToProjectTab(MobileTab.Terminal) }, modifier = Modifier.size(40.dp)) {
                    Icon(TablerIcons.Outline.Terminal2, contentDescription = "Open terminal", tint = Color.White)
                }
            },
        )
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                loadingMessages && !hasMessages -> ChatScreenSkeleton()
                !hasMessages && !isStreaming -> EmptyState(title = "Start the conversation", description = "Ask anything about your project.", icon = { Icon(TablerIcons.Outline.Message, contentDescription = null, tint = ConsoleColors.TextMuted) })
                else -> {
                    LazyColumn(state = listState, modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
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
                    }
                    EdgeScrollIndicator(
                        state = listState,
                        modifier = Modifier.align(Alignment.CenterEnd).padding(end = 2.dp),
                    )
                }
            }
            if (showScrollBottom) {
                androidx.compose.material3.FloatingActionButton(
                    onClick = { scope.launch { try { listState.animateScrollToItem(maxOf(0, displayMessages.size - 1)) } catch (_: Exception) {} } },
                    modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 16.dp),
                    containerColor = ConsoleColors.SurfaceElevated,
                    contentColor = ConsoleColors.TextPrimary,
                ) {
                    Icon(TablerIcons.Outline.ChevronDown, contentDescription = "Scroll to bottom")
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
                        keyboardController?.hide()
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
    @Suppress("UNUSED_EXPRESSION")
    LaunchedEffect(sessionStatuses[sessionId]) { }
}
