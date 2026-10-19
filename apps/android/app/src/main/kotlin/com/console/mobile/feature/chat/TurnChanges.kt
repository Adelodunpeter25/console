package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.util.getFileName
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Minus
import io.github.lyxnx.compose.ui.tablericons.outline.Plus

/** File-change chips under an assistant turn, folding after four files. */
@Composable
fun TurnChangeChip(
    change: SessionFileChange,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val name = remember(change.path) { getFileName(change.path) }
    val isDeleted = change.status == "deleted"
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(NewTheme.Card)
            .border(1.dp, NewTheme.Divider, RoundedCornerShape(6.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FileIcon(filename = name, sizeDp = 13, modifier = Modifier.padding(end = 6.dp))
        Text(
            text = name,
            color = if (isDeleted) NewTheme.TextMuted else NewTheme.TextPrimary,
            fontSize = 12.sp,
            fontFamily = ConsoleMonoFamily,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textDecoration = if (isDeleted) TextDecoration.LineThrough else null,
        )
        if (change.additions > 0 || change.deletions > 0) {
            Spacer(modifier = Modifier.size(6.dp))
            DiffSummaryBadge(addedCount = change.additions, removedCount = change.deletions)
        }
    }
}

@Composable
fun MoreChangesChip(
    expanded: Boolean,
    remainingCount: Int,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .clip(RoundedCornerShape(6.dp))
            .background(NewTheme.Card)
            .border(1.dp, NewTheme.Divider, RoundedCornerShape(6.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = if (expanded) TablerIcons.Outline.Minus else TablerIcons.Outline.Plus,
            contentDescription = null,
            tint = NewTheme.TextSecondary,
            modifier = Modifier.size(13.dp).padding(end = 4.dp),
        )
        Text(
            text = if (expanded) "Show less" else "$remainingCount more",
            color = NewTheme.TextSecondary,
            fontSize = 12.sp,
            fontFamily = ConsoleMonoFamily,
            fontWeight = FontWeight.Medium,
        )
    }
}
