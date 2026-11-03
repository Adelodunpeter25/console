package com.console.mobile.feature.home

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.SkeletonBlock

/**
 * Loading placeholder for the Home list. It mirrors [SessionSectionHeader] and
 * [SessionRow] measurement for measurement (same paddings, a 28dp first line,
 * a second line sized by the real 13sp text) so nothing moves when the
 * sessions arrive. Keep the two in step if either layout changes.
 */
@Composable
internal fun HomeSkeleton(sections: List<Int> = listOf(3, 2)) {
    Column(modifier = Modifier.fillMaxWidth()) {
        sections.forEachIndexed { sIdx, rows ->
            // Same box as SessionSectionHeader: its 36dp new-chat button row plus the
            // 4dp (first) / 14dp top and 4dp bottom padding around it.
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(start = 12.dp, end = 4.dp, top = if (sIdx == 0) 4.dp else 14.dp, bottom = 4.dp)
                    .height(36.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                SkeletonBlock(width = 96.dp, height = 14.dp, radius = 4.dp)
            }
            SectionCard(modifier = Modifier.padding(bottom = 10.dp)) {
                repeat(rows) { index ->
                    SkeletonSessionRow()
                    if (index < rows - 1) SectionDivider()
                }
            }
        }
    }
}

/** One placeholder row, laid out exactly like [SessionRow]. */
@Composable
private fun SkeletonSessionRow() {
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
            Box(modifier = Modifier.height(28.dp), contentAlignment = Alignment.CenterStart) {
                SkeletonBlock(modifier = Modifier.fillMaxWidth(0.6f), height = 16.dp)
            }
            SizedByText(fontSize = 13, modifier = Modifier.padding(top = 1.dp)) {
                SkeletonBlock(modifier = Modifier.fillMaxWidth(0.4f), height = 11.dp, radius = 4.dp)
            }
        }
        Column(horizontalAlignment = Alignment.End) {
            Box(modifier = Modifier.height(28.dp), contentAlignment = Alignment.CenterEnd) {
                SkeletonBlock(width = 52.dp, height = 20.dp, radius = 10.dp)
            }
            SizedByText(fontSize = 12, modifier = Modifier.padding(top = 1.dp)) {
                SkeletonBlock(width = 24.dp, height = 10.dp, radius = 4.dp)
            }
        }
    }
}

/**
 * Takes the height of one line of text at [fontSize] — including the user's font
 * scale — so a placeholder stands in for a text line without guessing its height.
 */
@Composable
private fun SizedByText(
    fontSize: Int,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit,
) {
    Box(modifier = modifier, contentAlignment = Alignment.CenterStart) {
        Text(" ", fontSize = fontSize.sp, color = Color.Transparent, maxLines = 1, overflow = TextOverflow.Clip)
        content()
    }
}
