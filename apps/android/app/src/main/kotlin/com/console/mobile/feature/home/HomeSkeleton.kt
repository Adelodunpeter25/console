package com.console.mobile.feature.home

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.components.common.new.SkeletonBlock
import com.console.mobile.ui.theme.NewTheme

/**
 * Loading placeholder for the Home list, matching [WorkspacesLabel],
 * [SessionSectionHeader], and the flat [SessionRow] layout.
 */
@Composable
internal fun HomeSkeleton(sections: List<Int> = listOf(3, 2)) {
    val headerShape = RoundedCornerShape(12.dp)
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(top = 6.dp, bottom = 2.dp)
                .padding(horizontal = 8.dp, vertical = 6.dp),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            SkeletonBlock(width = 96.dp, height = 12.dp, radius = 4.dp)
        }
        sections.forEachIndexed { sIdx, rows ->
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(top = if (sIdx == 0) 4.dp else 10.dp, bottom = 4.dp)
                    .height(40.dp)
                    .clip(headerShape)
                    .background(NewTheme.SurfaceContainerLow)
                    .border(1.dp, NewTheme.CardBorder, headerShape)
                    .padding(horizontal = 12.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                SkeletonBlock(width = 14.dp, height = 14.dp, radius = 4.dp)
                SkeletonBlock(
                    width = 88.dp,
                    height = 14.dp,
                    radius = 4.dp,
                    modifier = Modifier.padding(start = 10.dp),
                )
            }
            repeat(rows) {
                SkeletonSessionRow()
            }
        }
    }
}

/** One placeholder row, laid out like the flat [SessionRow]. */
@Composable
private fun SkeletonSessionRow() {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = 10.dp)
            .padding(horizontal = 10.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier.padding(end = 10.dp).size(18.dp),
            contentAlignment = Alignment.Center,
        ) {
            SkeletonBlock(width = 8.dp, height = 8.dp, radius = 4.dp)
        }
        Box(modifier = Modifier.weight(1f).padding(end = 10.dp)) {
            SkeletonBlock(modifier = Modifier.fillMaxWidth(0.62f), height = 15.dp, radius = 5.dp)
        }
        SkeletonBlock(width = 24.dp, height = 11.dp, radius = 4.dp)
    }
}
