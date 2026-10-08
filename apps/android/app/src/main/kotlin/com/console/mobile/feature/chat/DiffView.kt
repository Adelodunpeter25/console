package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import com.console.mobile.core.util.DiffLineType
import com.console.mobile.core.util.DiffResult
import com.console.mobile.core.util.getFileName
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily

/**
 * Port of components/chat/tools/diff-view.tsx.
 * Unified diff with +/- gutters, collapse past 60 lines.
 *
 * Native rows on purpose: the previous Sora editor brought its own scroll
 * handling (which fought the parent scroll) and needed a fixed estimated
 * height (which left dead space when the estimate missed). Plain text rows
 * measure exactly and scroll with the parent. Add/remove signal survives
 * as row backgrounds; token-level highlighting does not.
 */
@Composable
fun DiffView(diff: DiffResult, filePath: String? = null, maxCollapsedLines: Int = 60) {
    var expanded by remember(filePath, diff.lines.size) { mutableStateOf(false) }
    Column(
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (!filePath.isNullOrBlank()) {
            Text(getFileName(filePath), color = NewTheme.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        }
        val visible = if (!expanded && diff.lines.size > maxCollapsedLines) diff.lines.take(maxCollapsedLines) else diff.lines
        Column(
            modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
        ) {
            visible.forEach { line ->
                val background = when (line.type) {
                    DiffLineType.Added -> ConsoleColors.Syntax.DiffAddedBg
                    DiffLineType.Removed -> ConsoleColors.Syntax.DiffRemovedBg
                    else -> Color.Transparent
                }
                Text(
                    text = when (line.type) {
                        DiffLineType.Added -> "+ ${line.text}"
                        DiffLineType.Removed -> "- ${line.text}"
                        else -> "  ${line.text}"
                    },
                    color = NewTheme.TextPrimary,
                    fontSize = 12.sp,
                    fontFamily = ConsoleMonoFamily,
                    softWrap = false,
                    modifier = Modifier.fillMaxWidth().background(background).padding(horizontal = 8.dp, vertical = 2.dp),
                )
            }
        }
        if (!expanded && diff.lines.size > maxCollapsedLines) {
            Row(modifier = Modifier.fillMaxWidth().clickable { expanded = true }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextSecondary)
                        Text("Show ${diff.lines.size - maxCollapsedLines} more lines", color = NewTheme.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }
        } else if (expanded && diff.lines.size > maxCollapsedLines) {
            Row(modifier = Modifier.fillMaxWidth().clickable { expanded = false }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(TablerIcons.Outline.ChevronUp, contentDescription = null, tint = NewTheme.TextSecondary)
                        Text("Show less", color = NewTheme.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }
        }
    }
}

@Composable
fun DiffSummaryBadge(addedCount: Int, removedCount: Int) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (addedCount > 0) Text("+$addedCount", color = Color(0xFF34D399), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold)
        if (removedCount > 0) Text("-$removedCount", color = Color(0xFFF87171), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
    }
}
