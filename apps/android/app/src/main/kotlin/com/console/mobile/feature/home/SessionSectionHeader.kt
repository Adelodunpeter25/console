package com.console.mobile.feature.home

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.GroupedProjectSection
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.Plus

/** Stable collapse key for a section: virtual sections by name, then project
 * id, with the no-project bucket as "general". */
internal fun sectionCollapseKey(section: GroupedProjectSection): String = when {
    section.projectName == "Pinned" -> "pinned"
    section.projectName == "Settled" -> "settled"
    section.projectId != null -> section.projectId
    else -> "general"
}

/** "My workspaces" label above Pinned and the project groups, matching desktop. */
@Composable
internal fun WorkspacesLabel(
    collapsed: Boolean,
    onToggle: () -> Unit,
) {
    val rotation by animateFloatAsState(
        targetValue = if (collapsed) -90f else 0f,
        label = "workspacesChevron",
    )
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = 6.dp, bottom = 2.dp)
            .clip(RoundedCornerShape(8.dp))
            .clickable(onClick = onToggle)
            .padding(horizontal = 8.dp, vertical = 6.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(
            "My workspaces",
            color = NewTheme.TextMuted,
            fontSize = 13.sp,
            fontWeight = FontWeight.SemiBold,
        )
        Icon(
            TablerIcons.Outline.ChevronDown,
            contentDescription = if (collapsed) "Expand workspaces" else "Collapse workspaces",
            tint = NewTheme.TextMuted,
            modifier = Modifier
                .size(14.dp)
                .graphicsLayer { rotationZ = rotation },
        )
    }
}

/**
 * Desktop-style collapsible section header: leading chevron, group name, and
 * a trailing "+" button for real projects.
 */
@Composable
internal fun SessionSectionHeader(
    section: GroupedProjectSection,
    isFirst: Boolean,
    collapsed: Boolean,
    onToggle: () -> Unit,
    onNewChat: (() -> Unit)?,
) {
    val isSearch = section.projectId == "search"
    val rotation by animateFloatAsState(
        targetValue = if (collapsed) -90f else 0f,
        label = "sectionChevron",
    )
    val shape = RoundedCornerShape(12.dp)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(top = if (isFirst) 4.dp else 10.dp, bottom = 4.dp)
            .height(40.dp)
            .clip(shape)
            .then(
                if (!isSearch) {
                    Modifier.background(Color(0xFF131315))
                } else {
                    Modifier
                }
            )
            .clickable(enabled = !isSearch, onClick = onToggle)
            .padding(start = 12.dp, end = 6.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            modifier = Modifier.weight(1f),
        ) {
            if (!isSearch) {
                Icon(
                    TablerIcons.Outline.ChevronDown,
                    contentDescription = null,
                    tint = NewTheme.TextGhost,
                    modifier = Modifier
                        .size(14.dp)
                        .graphicsLayer { rotationZ = rotation },
                )
            }
            Text(
                section.projectName,
                color = if (isSearch) NewTheme.TextPrimary else NewTheme.TextSecondary,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.padding(start = if (isSearch) 0.dp else 10.dp),
            )
        }
        if (onNewChat != null) {
            IconButton(onClick = onNewChat, modifier = Modifier.size(32.dp)) {
                Icon(
                    TablerIcons.Outline.Plus,
                    contentDescription = "New chat in ${section.projectName}",
                    tint = NewTheme.TextSecondary,
                    modifier = Modifier.size(18.dp),
                )
            }
        }
    }
}
