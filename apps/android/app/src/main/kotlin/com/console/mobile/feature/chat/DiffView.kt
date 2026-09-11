package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
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
import com.console.mobile.core.util.DiffLine
import com.console.mobile.core.util.DiffLineType
import com.console.mobile.core.util.DiffResult
import com.console.mobile.core.util.getFileName
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily

/**
 * Port of components/chat/tools/diff-view.tsx.
 * Unified diff with +/- gutters, line numbers, collapse past 60 lines.
 */
@Composable
fun DiffView(diff: DiffResult, filePath: String? = null, maxCollapsedLines: Int = 60) {
    var expanded by remember(filePath, diff.lines.size) { mutableStateOf(false) }
    val shape = RoundedCornerShape(12.dp)
    Column(
        modifier = Modifier.fillMaxWidth().clip(shape)
            .background(Color(0xFF101113))
            .border(1.dp, Color.White.copy(alpha = 0.1f), shape),
    ) {
        if (!filePath.isNullOrBlank()) {
            Text(getFileName(filePath), color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(horizontal = 12.dp, vertical = 8.dp))
        }
        val visible = if (!expanded && diff.lines.size > maxCollapsedLines) diff.lines.take(maxCollapsedLines) else diff.lines
        Box(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())) {
            Column(modifier = Modifier.padding(vertical = 4.dp)) {
                visible.forEachIndexed { i, line -> DiffLineRow(line = line, index = i) }
            }
        }
        if (!expanded && diff.lines.size > maxCollapsedLines) {
            Row(modifier = Modifier.fillMaxWidth().clickable { expanded = true }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextSecondary)
                        Text("Show ${diff.lines.size - maxCollapsedLines} more lines", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }
        } else if (expanded && diff.lines.size > maxCollapsedLines) {
            Row(modifier = Modifier.fillMaxWidth().clickable { expanded = false }.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(modifier = Modifier.weight(1f), contentAlignment = Alignment.Center) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Filled.ExpandLess, contentDescription = null, tint = ConsoleColors.TextSecondary)
                        Text("Show less", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }
        }
    }
}

@Composable
private fun DiffLineRow(line: DiffLine, index: Int) {
    val isAdded = line.type == DiffLineType.Added
    val isRemoved = line.type == DiffLineType.Removed
    val bg = when {
        isAdded -> Color(0xFF34D399).copy(alpha = 0.1f)
        isRemoved -> Color(0xFFF87171).copy(alpha = 0.1f)
        else -> Color.Transparent
    }
    val fg = when {
        isAdded -> Color(0xFF4ADE80)
        isRemoved -> Color(0xFFF87171)
        else -> Color(0xFFE4E4E7)
    }
    Row(modifier = Modifier.background(bg).padding(horizontal = 8.dp, vertical = 2.dp), verticalAlignment = Alignment.Top) {
        Text(line.oldLineNo?.toString() ?: "", color = ConsoleColors.TextMuted.copy(alpha = 0.4f), fontSize = 10.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.width(24.dp))
        Text(line.newLineNo?.toString() ?: "", color = ConsoleColors.TextMuted.copy(alpha = 0.4f), fontSize = 10.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.width(24.dp))
        Text(if (isAdded) "+" else if (isRemoved) "-" else " ", color = fg, fontSize = 10.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.Bold, modifier = Modifier.width(8.dp))
        Text(line.text, color = fg.copy(alpha = if (isRemoved) 0.75f else 1f), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, lineHeight = 17.sp, modifier = Modifier.padding(start = 8.dp))
    }
}

@Composable
fun DiffSummaryBadge(addedCount: Int, removedCount: Int) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (addedCount > 0) Text("+$addedCount", color = Color(0xFF34D399), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold)
        if (removedCount > 0) Text("-$removedCount", color = Color(0xFFF87171), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
    }
}
