package com.console.mobile.feature.home

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.GroupedProjectSection
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Pinned
import io.github.lyxnx.compose.ui.tablericons.outline.Plus

/** Stable collapse key for a section: virtual sections by name, then project
 * id, with the no-project bucket as "general". */
internal fun sectionCollapseKey(section: GroupedProjectSection): String = when {
    section.projectName == "Pinned" -> "pinned"
    section.projectName == "Drafts" -> "drafts"
    section.projectName == "Settled" -> "settled"
    section.projectId != null -> section.projectId
    else -> "general"
}

/** A section heading on Home: folder glyph (a straight pin for Pinned, a virtual group),
 * name, collapse chevron, and the new-chat button for real projects. Tapping
 * the label collapses/expands the sessions under the section, like desktop. */
@Composable
internal fun SessionSectionHeader(
    section: GroupedProjectSection,
    isFirst: Boolean,
    collapsed: Boolean,
    onToggle: () -> Unit,
    onNewChat: (() -> Unit)?,
) {
    val isVirtual = section.projectId == "pinned" || section.projectId == "settled"
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 12.dp, end = 4.dp, top = if (isFirst) 4.dp else 14.dp, bottom = 4.dp),
        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.weight(1f).clip(RoundedCornerShape(8.dp)).clickable(onClick = onToggle),
        ) {
            Icon(
                when {
                    section.projectId == "pinned" -> TablerIcons.Outline.Pinned
                    section.projectId == "settled" -> TablerIcons.Outline.Check
                    else -> TablerIcons.Outline.FolderOpen
                },
                contentDescription = null,
                tint = NewTheme.Accent,
                modifier = Modifier.size(17.dp),
            )
            Text(
                section.projectName,
                color = NewTheme.Accent,
                fontSize = 15.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
        if (onNewChat != null) {
            IconButton(onClick = onNewChat, modifier = Modifier.size(36.dp)) {
                Icon(TablerIcons.Outline.Plus, contentDescription = "New chat in ${section.projectName}", tint = NewTheme.TextSecondary, modifier = Modifier.size(20.dp))
            }
        }
    }
}
