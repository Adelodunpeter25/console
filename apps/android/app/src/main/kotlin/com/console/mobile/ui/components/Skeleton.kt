package com.console.mobile.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.ConsoleColors
import com.valentinilk.shimmer.shimmer

/** Port of components/common/skeleton.tsx using compose-shimmer. */
@Composable
fun SkeletonBox(
    modifier: Modifier = Modifier,
    width: Dp? = null,
    height: Dp = 16.dp,
    radius: Dp = 8.dp,
) {
    var m: Modifier = modifier.shimmer().clip(RoundedCornerShape(radius)).background(ConsoleColors.SurfaceElevated).height(height)
    if (width != null) m = m.width(width)
    else m = m.fillMaxWidth()
    Box(m)
}

@Composable
fun SessionRowSkeleton(isLast: Boolean = false) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f).padding(end = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            SkeletonBox(modifier = Modifier.fillMaxWidth(0.6f))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                SkeletonBox(modifier = Modifier.fillMaxWidth(0.25f), height = 12.dp, radius = 4.dp)
                SkeletonBox(modifier = Modifier.fillMaxWidth(0.2f), height = 12.dp, radius = 4.dp)
            }
        }
        Column(horizontalAlignment = Alignment.End, verticalArrangement = Arrangement.spacedBy(6.dp)) {
            SkeletonBox(width = 48.dp, height = 16.dp, radius = 24.dp)
            SkeletonBox(width = 32.dp, height = 10.dp, radius = 4.dp)
        }
    }
}

@Composable
fun ProjectSectionSkeleton(rows: Int = 3) {
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 24.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp).padding(bottom = 8.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            SkeletonBox(width = 96.dp, height = 14.dp, radius = 4.dp)
            SkeletonBox(width = 24.dp, height = 24.dp, radius = 8.dp)
        }
        androidx.compose.material3.Surface(
            color = ConsoleColors.Card,
            shape = RoundedCornerShape(16.dp),
        ) {
            Column {
                repeat(rows) { SkeletonBox(modifier = Modifier.padding(4.dp), height = 56.dp, radius = 12.dp) }
            }
        }
    }
}

@Composable
fun SessionListSkeleton() {
    Column(modifier = Modifier.padding(vertical = 4.dp)) {
        ProjectSectionSkeleton(rows = 3)
        ProjectSectionSkeleton(rows = 2)
    }
}

@Composable
fun ChatScreenSkeleton() {
    Column(modifier = Modifier.fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Column(modifier = Modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
            SkeletonBox(modifier = Modifier.fillMaxWidth(0.6f), height = 48.dp, radius = 20.dp)
        }
        Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth(0.85f)) {
            SkeletonBox(width = 112.dp)
            SkeletonBox(height = 80.dp, radius = 16.dp)
        }
        Column(modifier = Modifier.fillMaxWidth(), horizontalAlignment = Alignment.End) {
            SkeletonBox(modifier = Modifier.fillMaxWidth(0.4f), height = 40.dp, radius = 20.dp)
        }
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            SkeletonBox(height = 48.dp, radius = 12.dp)
            SkeletonBox(modifier = Modifier.fillMaxWidth(0.8f), height = 64.dp, radius = 16.dp)
        }
    }
}
