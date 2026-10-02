package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.IconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.snapshotFlow
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
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
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
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/** Scroll position to restore after older messages are prepended. */
private data class ListAnchor(
    val index: Int,
    val offset: Int,
    val sizeBefore: Int,
)

/**
 * Scrolls to the true end of the list. Jumping to the last item only aligns
 * its top with the viewport, which leaves a tall or growing last row's bottom
 * off screen, so ask for an oversized offset that the list clamps at the end.
 */
private suspend fun LazyListState.scrollToBottom() {
    val last = layoutInfo.totalItemsCount - 1
    if (last < 0) return
    // One call, so there's no idle gap between "jumped" and "reached the end"
    // for the drag/settle tracking to misread as the user leaving the bottom.
    scrollToItem(last, Int.MAX_VALUE)
}

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
            ScreenHeader(title = "Chat", centerTitle = false, onBack = { onBackToHome() })
            EmptyState(title = "No session selected", description = "Pick a chat from Home to get started.", icon = { Icon(TablerIcons.Outline.Message, contentDescription = null, tint = ConsoleColors.TextMuted) })
        }
        return
    }

    val chat: ChatSessionState = chatSessions[sessionId] ?: com.console.mobile.core.chat.createChatSessionState()
    var loadingMessages by remember(sessionId) { mutableStateOf(chat.messages.isEmpty()) }
    var overflowMenu by remember { mutableStateOf(false) }
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
        // loadDetail used to be fire-and-forget, so the status read after this
        // block came from the session list rather than from the detail response
        // and was routinely stale — the attach then never fired on a session
        // that was in fact running. Awaiting it makes the status below real.
        val header = coroutineScope {
            val detail = async { AppContainer.sessionRepository.loadDetail(sessionId) }
            launch { AppContainer.chatRepository.loadTodos(sessionId) }
            launch { AppContainer.chatRepository.loadSubagents(sessionId) }
            detail.await()
        }
        loadingMessages = false
        if (header?.status == SessionStatus.Working) {
            AppContainer.chatRepository.attachServerRun(sessionId)
        }
    }

    // A run can also start after entry — the desktop app, another device, or a
    // queued prompt — and the chat screen is where its output belongs. Watching
    // the status (rather than sampling it once) is what makes that show up live.
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

    val displaySize = displayMessages.size

    // Whether the list is pinned to the bottom. An explicit flag rather than
    // something inferred from the last visible index: the last row is a tall,
    // growing item (user message + run activity), so "last index visible" says
    // nothing about whether its bottom is on screen. The user taking over with
    // a drag turns it off; once scrolling settles it is re-derived from whether
    // there is anything left below, so scrolling back down resumes following.
    var following by remember(sessionId) { mutableStateOf(true) }
    LaunchedEffect(listState) {
        launch {
            listState.interactionSource.interactions.collect {
                if (it is DragInteraction.Start) following = false
            }
        }
        snapshotFlow { listState.isScrollInProgress }.collect { scrolling ->
            if (!scrolling) following = !listState.canScrollForward
        }
    }
    val showScrollBottom by remember { derivedStateOf { !following && listState.canScrollForward } }

    // Auto-follow while a run is active. Watches the list's real extent (row
    // count + the last row's size) so it re-fires as streamed text, tool calls
    // and run activity grow the bottom row — not just on discrete events. It
    // is keyed on layout, not the message count, so a pagination prepend that
    // adds nothing at the bottom doesn't count as growth the user should chase.
    val runningState by rememberUpdatedState(chat.running || isStreaming)
    LaunchedEffect(listState) {
        snapshotFlow {
            val info = listState.layoutInfo
            val last = info.visibleItemsInfo.lastOrNull()
            Triple(info.totalItemsCount, last?.index, last?.size)
        }.collect {
            if (following && runningState && !listState.isScrollInProgress) {
                try { listState.scrollToBottom() } catch (_: Exception) {}
            }
        }
    }

    // Opening a session lands on the newest page, so the list has to be jumped
    // to the bottom once that page arrives. The auto-follow above can't do it:
    // it's gated on an active run, and this case is an idle chat.
    // Keyed on the message count so it fires exactly once per entry — a later
    // prepend or append must not drag the user back down over their scroll.
    var settledInitialPage by remember(sessionId) { mutableStateOf(false) }
    LaunchedEffect(sessionId, displaySize) {
        if (settledInitialPage || displaySize <= 0) return@LaunchedEffect
        settledInitialPage = true
        following = true
        try { listState.scrollToBottom() } catch (_: Exception) {}
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
        // Keep auto-follow from racing the anchor restore once the page lands.
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

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = chatTitle,
            centerTitle = false,
            onBack = {
                AppContainer.appStateHolder.setActiveTab(MobileTab.Home)
                onBackToHome()
            },
            actions = {
                IconButton(onClick = { jumpToProjectTab(MobileTab.Files) }, modifier = Modifier.size(40.dp)) {
                    Icon(TablerIcons.Outline.Files, contentDescription = "Open file explorer", tint = Color.White)
                }
                Box {
                    IconButton(onClick = { overflowMenu = true }, modifier = Modifier.size(40.dp)) {
                        Icon(TablerIcons.Outline.DotsVertical, contentDescription = "More options", tint = Color.White)
                    }
                    DropdownMenu(
                        expanded = overflowMenu,
                        onDismissRequest = { overflowMenu = false },
                        modifier = Modifier.background(ConsoleColors.SurfaceElevated),
                    ) {
                        DropdownMenuItem(
                            text = { Text("Open diff", color = ConsoleColors.TextPrimary) },
                            leadingIcon = { Icon(TablerIcons.Outline.BrandGit, contentDescription = null, tint = ConsoleColors.TextPrimary) },
                            onClick = {
                                overflowMenu = false
                                jumpToProjectTab(MobileTab.Changes)
                            },
                        )
                        DropdownMenuItem(
                            text = { Text("Open terminal", color = ConsoleColors.TextPrimary) },
                            leadingIcon = { Icon(TablerIcons.Outline.Terminal2, contentDescription = null, tint = ConsoleColors.TextPrimary) },
                            onClick = {
                                overflowMenu = false
                                jumpToProjectTab(MobileTab.Terminal)
                            },
                        )
                    }
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
                    onClick = {
                        following = true
                        scope.launch { try { listState.scrollToBottom() } catch (_: Exception) {} }
                    },
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
                        following = true
                        scope.launch { try { listState.scrollToBottom() } catch (_: Exception) {} }
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
