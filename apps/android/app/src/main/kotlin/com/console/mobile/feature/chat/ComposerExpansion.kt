package com.console.mobile.feature.chat

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.Stable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.unit.Constraints
import kotlin.math.roundToInt

/**
 * Keeps the composer expanded while something it launched is open.
 *
 * The composer collapses to a one-line pill when it loses focus. Opening a
 * model/branch/approval sheet or the image picker moves window focus (and hides
 * the keyboard), which would otherwise read as "the user left" and collapse the
 * composer from under the thing they are in the middle of using.
 */
@Stable
class ComposerHold {
    private var count by mutableIntStateOf(0)
    val held: Boolean get() = count > 0
    fun acquire() { count++ }
    fun release() { if (count > 0) count-- }
}

val LocalComposerHold = staticCompositionLocalOf<ComposerHold?> { null }

/** Holds the composer open for as long as [active] is true. */
@Composable
fun HoldComposerOpen(active: Boolean) {
    val hold = LocalComposerHold.current
    DisposableEffect(active, hold) {
        if (active) hold?.acquire()
        onDispose { if (active) hold?.release() }
    }
}

/**
 * Lays [content] out at [progress] (0..1) of its natural height, clipped and
 * faded. Unlike AnimatedVisibility it never leaves composition, so state living
 * inside — a sheet that is open, a loaded branch list — survives the composer
 * collapsing and re-expanding. [anchorBottom] keeps the bottom edge fixed while
 * it shrinks (for a strip sitting above the input), otherwise the top edge.
 */
@Composable
fun Collapsible(
    progress: Float,
    modifier: Modifier = Modifier,
    anchorBottom: Boolean = false,
    content: @Composable () -> Unit,
) {
    val p = progress.coerceIn(0f, 1f)
    Layout(content = content, modifier = modifier.clipToBounds().alpha(p)) { measurables, constraints ->
        val loose = constraints.copy(minHeight = 0, maxHeight = Constraints.Infinity)
        val placeables = measurables.map { it.measure(loose) }
        val width = placeables.maxOfOrNull { it.width } ?: 0
        val full = placeables.sumOf { it.height }
        val height = (full * p).roundToInt()
        layout(width, height) {
            var y = if (anchorBottom) height - full else 0
            placeables.forEach {
                it.placeRelative(0, y)
                y += it.height
            }
        }
    }
}

/** Keyboard height assumed before one has been seen, so the first-ever open still tracks smoothly. */
const val DEFAULT_IME_HEIGHT_DP = 280

/**
 * How far open the composer is for a given keyboard height, 0..1. Driven by the
 * keyboard's own animated inset so the composer moves in lockstep with it —
 * a separate timer made the two run one after the other.
 */
fun imeExpansion(imeBottomPx: Int, referencePx: Int): Float {
    if (imeBottomPx <= 0 || referencePx <= 0) return 0f
    return (imeBottomPx.toFloat() / referencePx).coerceIn(0f, 1f)
}
