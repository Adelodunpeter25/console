package com.console.mobile.feature.chat

import androidx.compose.foundation.lazy.LazyListState

/** Scroll position to restore after older messages are prepended. */
internal data class ListAnchor(
    val index: Int,
    val offset: Int,
    val sizeBefore: Int,
)

/**
 * Scrolls to the true end of the list. Jumping to the last item only aligns
 * its top with the viewport, which leaves a tall or growing last row's bottom
 * off screen, so ask for an oversized offset that the list clamps at the end.
 */
internal suspend fun LazyListState.scrollToBottom() {
    val last = layoutInfo.totalItemsCount - 1
    if (last < 0) return
    scrollToItem(last, Int.MAX_VALUE)
}
