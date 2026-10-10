package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
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
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import com.console.mobile.core.util.DiffLine
import com.console.mobile.core.util.DiffLineType
import com.console.mobile.core.util.DiffResult
import com.console.mobile.core.util.getFileName
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily

/** Subtle row wash matching desktop's hsla values (0.08 alpha) */
private val AddedLineBg = Color(0x1534D399)
private val RemovedLineBg = Color(0x15F87171)

/**
 * Port of desktop's `DiffView` (`apps/desktop/.../chat/diff_view.rs`).
 * 3-column layout: line number gutter, +/- symbol, and syntax-colored text.
 */
@Composable
fun DiffView(
    diff: DiffResult,
    filePath: String? = null,
    maxCollapsedLines: Int = 60,
    showFileHeader: Boolean = true,
) {
    var expanded by remember(filePath, diff.lines.size) { mutableStateOf(false) }
    Column(
        modifier = Modifier.fillMaxWidth(),
    ) {
        if (showFileHeader && !filePath.isNullOrBlank()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp, vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                FileIcon(filename = filePath, sizeDp = 13, modifier = Modifier.padding(end = 6.dp))
                Text(
                    text = getFileName(filePath),
                    color = NewTheme.TextSecondary,
                    fontSize = 11.sp,
                    fontFamily = ConsoleMonoFamily,
                    fontWeight = FontWeight.Medium,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                DiffSummaryBadge(addedCount = diff.addedCount, removedCount = diff.removedCount)
            }
        }
        val visible = if (!expanded && diff.lines.size > maxCollapsedLines) diff.lines.take(maxCollapsedLines) else diff.lines
        Column(
            modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
        ) {
            visible.forEach { line ->
                DiffLineRow(line)
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
private fun DiffLineRow(line: DiffLine) {
    val (gutter, fg, bg) = when (line.type) {
        DiffLineType.Added -> Triple("+", NewTheme.Success, AddedLineBg)
        DiffLineType.Removed -> Triple("-", NewTheme.Danger, RemovedLineBg)
        DiffLineType.Context -> Triple(" ", NewTheme.TextSecondary, Color.Transparent)
    }
    val lineNo = when (line.type) {
        DiffLineType.Added -> line.newLineNo?.toString() ?: ""
        DiffLineType.Removed -> line.oldLineNo?.toString() ?: ""
        DiffLineType.Context -> line.newLineNo?.toString() ?: ""
    }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .background(bg)
            .padding(horizontal = 8.dp, vertical = 1.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Column 1: Line number gutter (matching desktop 32dp width)
        Text(
            text = lineNo,
            color = NewTheme.TextGhost,
            fontSize = 10.sp,
            fontFamily = ConsoleMonoFamily,
            textAlign = TextAlign.End,
            modifier = Modifier.width(32.dp),
        )
        // Column 2: Sign indicator (+ / - / space)
        Text(
            text = gutter,
            color = fg,
            fontSize = 11.sp,
            fontFamily = ConsoleMonoFamily,
            fontWeight = FontWeight.Bold,
            textAlign = TextAlign.Center,
            modifier = Modifier.width(16.dp),
        )
        // Column 3: Code text with matching color
        Text(
            text = line.text.ifEmpty { " " },
            color = fg,
            fontSize = 12.sp,
            fontFamily = ConsoleMonoFamily,
            lineHeight = 17.sp,
            softWrap = false,
        )
    }
}

@Composable
fun DiffSummaryBadge(addedCount: Int, removedCount: Int) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (addedCount > 0) Text("+$addedCount", color = Color(0xFF34D399), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold)
        if (removedCount > 0) Text("-$removedCount", color = Color(0xFFF87171), fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
    }
}
