package com.console.mobile.feature.chat

import androidx.compose.foundation.interaction.DragInteraction
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import com.console.mobile.core.util.ScrollMemory
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.drop
import kotlinx.coroutines.launch

/**
 * Scroll state for one chat transcript: where the list is, whether new
 * messages should keep following it, and the viewport anchor used when older
 * messages are prepended.
 */
@Stable
internal class ChatTranscriptScroll(
    val listState: LazyListState,
    following: Boolean,
    settled: Boolean,
) {
    var following by mutableStateOf(following)
        private set
    var settledInitialPage by mutableStateOf(settled)
        private set

    internal var anchor by mutableStateOf<ListAnchor?>(null)
    internal var autoLoadBlocked by mutableStateOf(false)
        private set

    val showScrollToBottom: Boolean
        get() = !following && listState.canScrollForward

    /** Jump to the newest message and resume following the run. */
    fun scrollToBottom(scope: CoroutineScope) {
        following = true
        scope.launch { try { listState.scrollToBottom() } catch (_: Exception) {} }
    }

    internal fun stopFollowing() {
        following = false
    }

    internal fun followBottomIfAtEnd() {
        following = !listState.canScrollForward
    }

    internal fun settleAtBottom() {
        settledInitialPage = true
        following = true
    }

    internal fun captureAnchor(sizeBefore: Int) {
        anchor = ListAnchor(
            index = listState.firstVisibleItemIndex,
            offset = listState.firstVisibleItemScrollOffset,
            sizeBefore = sizeBefore,
        )
    }

    internal fun clearAnchor() {
        anchor = null
    }

    internal fun resetAutoLoad() {
        autoLoadBlocked = false
    }

    internal fun blockAutoLoad() {
        autoLoadBlocked = true
    }
}

/**
 * Restores this session's remembered position, then keeps it up to date and
 * handles follow-the-latest, the initial jump to the bottom, and pagination
 * anchoring. The screen only has to render the list and ask to scroll down.
 */
@Composable
internal fun rememberChatTranscriptScroll(
    sessionId: String,
    messagesLoaded: Boolean,
    displaySize: Int,
    hasMoreMessages: Boolean,
    loadingOlder: Boolean,
    loadOlder: () -> Boolean,
    onDragStarted: () -> Unit,
): ChatTranscriptScroll {
    // Only trust the remembered position when the messages are still loaded;
    // otherwise a fresh load should start at the bottom as usual.
    val restored = remember(sessionId) {
        ScrollMemory.get(sessionId)?.takeIf { messagesLoaded }
    }
    val listState = key(sessionId) {
        rememberLazyListState(
            initialFirstVisibleItemIndex = restored?.index ?: 0,
            initialFirstVisibleItemScrollOffset = restored?.offset ?: 0,
        )
    }
    val scroll = remember(sessionId, listState) {
        ChatTranscriptScroll(
            listState = listState,
            following = restored?.atEnd ?: true,
            settled = restored != null,
        )
    }

    // Record the position as it moves. Saving only on dispose would race the
    // next screen: during a navigation transition both are composed, so the
    // incoming chat could read a stale entry.
    LaunchedEffect(sessionId, scroll) {
        snapshotFlow {
            Triple(
                scroll.listState.firstVisibleItemIndex,
                scroll.listState.firstVisibleItemScrollOffset,
                !scroll.listState.canScrollForward,
            )
        }.collect { (index, offset, atEnd) ->
            ScrollMemory.save(sessionId, index, offset, atEnd)
        }
    }

    val dragStarted = rememberUpdatedState(onDragStarted)
    LaunchedEffect(scroll) {
        launch {
            scroll.listState.interactionSource.interactions.collect {
                if (it is DragInteraction.Start) {
                    scroll.stopFollowing()
                    dragStarted.value()
                }
            }
        }
        // The initial `false` is not a scroll ending — dropping it keeps a
        // restored mid-transcript position from being treated as "at bottom"
        // before the list has even been measured.
        snapshotFlow { scroll.listState.isScrollInProgress }.drop(1).collect { scrolling ->
            if (!scrolling) scroll.followBottomIfAtEnd()
        }
    }

    LaunchedEffect(scroll) {
        snapshotFlow {
            val info = scroll.listState.layoutInfo
            val last = info.visibleItemsInfo.lastOrNull()
            Triple(info.totalItemsCount, last?.index, last?.size)
        }.collect {
            if (scroll.following && !scroll.listState.isScrollInProgress) {
                try { scroll.listState.scrollToBottom() } catch (_: Exception) {}
            }
        }
    }

    LaunchedEffect(sessionId, displaySize, scroll) {
        if (scroll.settledInitialPage || displaySize <= 0) return@LaunchedEffect
        scroll.settleAtBottom()
        try { scroll.listState.scrollToBottom() } catch (_: Exception) {}
    }

    // Older-message pagination.
    val atTop = scroll.listState.firstVisibleItemIndex <= 0
    val loadOlderNow = rememberUpdatedState(loadOlder)
    LaunchedEffect(scroll, atTop, hasMoreMessages, loadingOlder, displaySize) {
        if (!atTop) {
            scroll.resetAutoLoad()
            return@LaunchedEffect
        }
        if (scroll.autoLoadBlocked || !hasMoreMessages || loadingOlder || displaySize <= 0) return@LaunchedEffect
        scroll.captureAnchor(displaySize)
        if (scroll.listState.canScrollForward) scroll.stopFollowing()
        if (!loadOlderNow.value()) scroll.blockAutoLoad()
    }

    LaunchedEffect(scroll, displaySize) {
        val anchor = scroll.anchor ?: return@LaunchedEffect
        val delta = displaySize - anchor.sizeBefore
        if (delta > 0) {
            try { scroll.listState.scrollToItem(anchor.index + delta, anchor.offset) } catch (_: Exception) {}
        }
        scroll.clearAnchor()
    }

    return scroll
}
