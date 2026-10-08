package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Robot
import console.v1.SubagentInfo
import console.v1.TodoItem
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.OptionRow
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TintPill
import com.console.mobile.ui.theme.NewTheme

fun todoCounts(items: List<TodoItem>): Pair<Int, Int> {
    val done = items.count { it.status == "completed" || it.status == "done" || it.status == "complete" }
    return done to items.size
}

fun nextPendingTodo(items: List<TodoItem>): TodoItem? =
    items.firstOrNull { it.status == "in_progress" }
        ?: items.firstOrNull { it.status != "completed" && it.status != "done" && it.status != "complete" }

/** Port of todo-banner.tsx + subagent-banner.tsx. Collapsed strip above composer / interaction panel. */
@Composable
fun TodoBanner(completed: Int, total: Int, nextTask: String?, onPress: () -> Unit) {
    BannerShell(label = "TASKS", count = "$completed/$total", detail = nextTask?.let { "• $it" }, onPress = onPress)
}

@Composable
fun SubagentBanner(subagents: List<SubagentInfo>, onPress: () -> Unit) {
    val latest = subagents.lastOrNull()
    val statusLabel = when (latest?.status) {
        "running" -> if ((latest.max_turns) > 0) "Running (Turn ${maxOf(1, latest.current_turn)}/${latest.max_turns})" else "Running"
        "completed" -> "Done"
        "aborted" -> "Aborted"
        null -> ""
        else -> "Failed"
    }
    val detail = latest?.let { "• ${it.role} ($statusLabel)" }
    BannerShell(label = "SUBAGENTS", count = "${subagents.size}", detail = detail, onPress = onPress, running = subagents.any { it.status == "running" })
}

@Composable
private fun BannerShell(label: String, count: String, detail: String?, onPress: () -> Unit, running: Boolean = false) {
    // A raised card strip above the composer. The icon takes the accent while work is running.
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 10.dp)
            .clip(RoundedCornerShape(NewTheme.FieldRadius))
            .background(NewTheme.Card)
            .clickable(onClick = onPress)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            if (label == "SUBAGENTS") TablerIcons.Outline.Robot else TablerIcons.Outline.Check,
            contentDescription = null,
            tint = if (running) NewTheme.Accent else NewTheme.TextSecondary,
            modifier = Modifier.size(18.dp),
        )
        Text(label, color = NewTheme.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(start = 10.dp))
        Text(
            count, color = NewTheme.TextSecondary, fontSize = 11.sp, fontWeight = FontWeight.SemiBold,
            modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(999.dp)).background(Color.White.copy(alpha = 0.08f)).padding(horizontal = 8.dp, vertical = 2.dp),
        )
        if (detail != null) {
            Text(detail, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 10.dp))
        } else {
            androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
        }
        Icon(TablerIcons.Outline.ChevronUp, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(18.dp))
    }
}

@Composable
fun TodoBottomSheet(items: List<TodoItem>, completed: Int, total: Int, onDismiss: () -> Unit) {
    BaseSheet(onDismiss = onDismiss, title = "Tasks ($completed/$total)") {
        SectionCard {
            items.forEachIndexed { index, item ->
                val done = item.status == "completed" || item.status == "done" || item.status == "complete"
                val inProgress = item.status == "in_progress"
                Row(
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    // Status marker: filled check when done, accent dot while running, empty ring otherwise.
                    Box(
                        modifier = Modifier.size(22.dp).clip(CircleShape)
                            .background(if (done) NewTheme.Success.copy(alpha = 0.18f) else if (inProgress) NewTheme.Accent.copy(alpha = 0.18f) else Color.White.copy(alpha = 0.06f)),
                        contentAlignment = Alignment.Center,
                    ) {
                        if (done) Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(13.dp))
                        else if (inProgress) Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(NewTheme.Accent))
                    }
                    Text(
                        item.content, fontSize = 16.sp, lineHeight = 22.sp,
                        color = if (done) NewTheme.TextMuted else NewTheme.TextPrimary,
                        fontWeight = if (inProgress) FontWeight.Medium else FontWeight.Normal,
                        textDecoration = if (done) TextDecoration.LineThrough else null,
                        modifier = Modifier.weight(1f).padding(start = 14.dp),
                    )
                    if (inProgress) TintPill("In progress", NewTheme.Accent, Modifier.padding(start = 8.dp))
                    else if (done) TintPill("Done", NewTheme.Success, Modifier.padding(start = 8.dp))
                }
                if (index < items.lastIndex) SectionDivider(startInset = 54.dp)
            }
        }
    }
}

@Composable
fun SubagentSheet(subagents: List<SubagentInfo>, selectedId: String?, onSelect: (String?) -> Unit, onDismiss: () -> Unit, onOpenDetails: (String) -> Unit) {
    var selected by remember(selectedId) { mutableStateOf(selectedId) }
    BaseSheet(onDismiss = onDismiss, title = "Subagents (${subagents.size})") {
        SectionCard {
            subagents.forEachIndexed { index, s ->
                val dot = when (s.status) {
                    "running" -> NewTheme.Gauge
                    "completed" -> NewTheme.Success
                    else -> NewTheme.Danger
                }
                OptionRow(
                    title = s.name.ifBlank { s.role.ifBlank { "Subagent" } },
                    subtitle = "${s.role} · ${s.status}",
                    selected = s.subagent_id == selected,
                    leading = { Box(modifier = Modifier.size(10.dp).clip(CircleShape).background(dot)) },
                ) {
                    selected = s.subagent_id
                    onSelect(s.subagent_id)
                    onOpenDetails(s.subagent_id)
                }
                if (index < subagents.lastIndex) SectionDivider(startInset = 42.dp)
            }
        }
    }
}
