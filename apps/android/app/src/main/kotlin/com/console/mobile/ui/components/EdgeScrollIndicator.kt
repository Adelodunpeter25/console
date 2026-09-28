package com.console.mobile.ui.components

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.width
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.delay

/**
 * Transient edge scroll indicator: a thin thumb that appears while the user is
 * scrolling and fades out shortly after they stop.
 *
 * Compose ships an equivalent (`Modifier.verticalScrollbar` /
 * `LinearScrollIndicator`), but neither is present in the Foundation and
 * Material3 artifacts this app resolves, so this is hand-rolled to match the
 * behaviour. Content height is estimated from the mean size of currently
 * visible items, which is exact at both ends of the list and approximate in
 * between — good enough for a scrollbar on a list of uneven message heights.
 */
@Composable
fun EdgeScrollIndicator(
    state: LazyListState,
    modifier: Modifier = Modifier,
    thickness: Dp = 3.dp,
    color: Color = ConsoleColors.TextMuted.copy(alpha = 0.5f),
    hideDelayMillis: Long = 500L,
) {
    var scrolling by remember { mutableStateOf(false) }
    LaunchedEffect(state) {
        snapshotFlow { state.isScrollInProgress }.collect { scrolling = it }
    }
    // Hold the thumb on screen briefly after the scroll ends so it doesn't
    // vanish the instant the user's finger lifts.
    var shown by remember { mutableStateOf(false) }
    LaunchedEffect(scrolling) {
        if (scrolling) shown = true else {
            delay(hideDelayMillis)
            shown = false
        }
    }
    val alpha by animateFloatAsState(if (shown) 1f else 0f, label = "edgeScrollIndicator")
    if (alpha <= 0.01f) return

    val metrics by remember(state) {
        derivedStateOf { scrollMetrics(state) }
    }

    Canvas(modifier = modifier.width(thickness).fillMaxHeight()) {
        val track = size.height
        if (track <= 0f || !metrics.scrollable) return@Canvas
        val thumbHeight = (track * metrics.heightFraction).coerceIn(
            minimumValue = size.width * 4f,
            maximumValue = track,
        )
        val thumbTop = (track - thumbHeight) * metrics.positionFraction
        drawRoundRect(
            color = color.copy(alpha = color.alpha * alpha),
            topLeft = Offset(0f, thumbTop),
            size = Size(size.width, thumbHeight),
            cornerRadius = CornerRadius(size.width / 2f, size.width / 2f),
        )
    }
}

private data class ScrollMetrics(
    val scrollable: Boolean,
    val positionFraction: Float,
    val heightFraction: Float,
)

/** Where the thumb sits and how big it is, as a fraction of the track. */
private fun scrollMetrics(state: LazyListState): ScrollMetrics {
    val info = state.layoutInfo
    val items = info.visibleItemsInfo
    val viewport = info.viewportEndOffset - info.viewportStartOffset
    if (items.isEmpty() || viewport <= 0 || info.totalItemsCount <= 0) {
        return ScrollMetrics(false, 0f, 1f)
    }
    val first = items.first()
    val last = items.last()
    val visibleHeight = (last.offset + last.size - first.offset).toFloat()
    if (visibleHeight <= 0f) return ScrollMetrics(false, 0f, 1f)

    val average = visibleHeight / items.size
    val totalHeight = average * info.totalItemsCount
    val maxScroll = totalHeight - viewport
    if (maxScroll <= 0f) return ScrollMetrics(false, 0f, 1f)

    // Normalise over the scrollable range, not the whole content height, or the
    // thumb parks short of the bottom edge instead of against it.
    val scrolled = average * first.index + state.firstVisibleItemScrollOffset
    return ScrollMetrics(
        scrollable = true,
        positionFraction = (scrolled / maxScroll).coerceIn(0f, 1f),
        heightFraction = (visibleHeight / totalHeight).coerceIn(0f, 1f),
    )
}
