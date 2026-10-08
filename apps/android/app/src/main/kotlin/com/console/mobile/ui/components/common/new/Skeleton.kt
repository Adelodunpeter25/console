package com.console.mobile.ui.components.common.new

import androidx.compose.animation.Crossfade
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme
import com.valentinilk.shimmer.shimmer

/** A shimmering placeholder block. Full width unless [width] is given. */
@Composable
fun SkeletonBlock(
    modifier: Modifier = Modifier,
    width: Dp? = null,
    height: Dp = 16.dp,
    radius: Dp = 8.dp,
) {
    var m: Modifier = modifier.shimmer().clip(RoundedCornerShape(radius)).background(NewTheme.Skeleton).height(height)
    m = if (width != null) m.width(width) else m.fillMaxWidth()
    Box(m)
}

/**
 * A [Section]-shaped placeholder: a heading bar over a rounded card of [rows]
 * rows, each an icon circle with two lines of text. Matches the real section's
 * proportions so the page doesn't jump when content arrives.
 */
@Composable
fun SectionSkeleton(rows: Int = 3, modifier: Modifier = Modifier) {
    Column(modifier = modifier.fillMaxWidth()) {
        SkeletonBlock(width = 96.dp, height = 14.dp, radius = 4.dp, modifier = Modifier.padding(start = 12.dp, top = 22.dp, bottom = 8.dp))
        Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card)) {
            repeat(rows) { index ->
                Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                    SkeletonBlock(width = 24.dp, height = 24.dp, radius = 12.dp)
                    Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
                        SkeletonBlock(modifier = Modifier.fillMaxWidth(0.55f), height = 16.dp)
                        SkeletonBlock(modifier = Modifier.fillMaxWidth(0.35f).padding(top = 0.dp), height = 12.dp, radius = 4.dp)
                    }
                }
                if (index < rows - 1) SectionDivider(startInset = 58.dp)
            }
        }
    }
}

/**
 * Shows [skeleton] while [loading], then fades to [content]. The wrapper owns the
 * switch so screens don't each hand-roll an if/else around their loading flag.
 */
@Composable
fun SkeletonLoader(
    loading: Boolean,
    modifier: Modifier = Modifier,
    skeleton: @Composable () -> Unit = { SectionSkeleton() },
    content: @Composable () -> Unit,
) {
    Crossfade(targetState = loading, modifier = modifier, animationSpec = tween(150), label = "skeletonLoader") { isLoading ->
        if (isLoading) skeleton() else content()
    }
}
