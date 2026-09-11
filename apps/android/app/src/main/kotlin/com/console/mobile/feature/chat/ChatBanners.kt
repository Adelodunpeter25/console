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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.SmartToy
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
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.data.model.TodoItem
import com.console.mobile.ui.theme.ConsoleColors

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
        "running" -> if ((latest.maxTurns) > 0) "Running (Turn ${maxOf(1, latest.currentTurn)}/${latest.maxTurns})" else "Running"
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
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 10.dp).clip(RoundedCornerShape(12.dp))
            .background(Color(0xFF121214))
            .border(1.dp, Color(0xFF27272A), RoundedCornerShape(12.dp))
            .clickable(onClick = onPress)
            .padding(horizontal = 14.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(modifier = Modifier.size(24.dp).clip(RoundedCornerShape(6.dp)).background(Color(0xFF1C1C20).copy(alpha = 1f)).border(1.dp, Color(0xFF303036), RoundedCornerShape(6.dp)), contentAlignment = Alignment.Center) {
            Icon(if (label == "SUBAGENTS") Icons.Filled.SmartToy else Icons.Filled.Check, contentDescription = null, tint = if (running) Color(0xFF38BDF8) else ConsoleColors.TextSecondary, modifier = Modifier.size(13.dp))
        }
        Text(label, color = Color(0xFFFAFAFA), fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(start = 10.dp))
        Box(modifier = Modifier.padding(start = 6.dp).clip(RoundedCornerShape(6.dp)).background(Color(0xFF222226)).border(1.dp, Color(0xFF33333A), RoundedCornerShape(6.dp)).padding(horizontal = 6.dp, vertical = 2.dp)) {
            Text(count, color = ConsoleColors.TextSecondary, fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
        }
        if (detail != null) {
            Text(detail, color = ConsoleColors.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
        } else {
            androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
        }
        Box(modifier = Modifier.size(20.dp).clip(CircleShape).background(Color(0xFF1C1C20)).border(1.dp, Color(0xFF303036), CircleShape), contentAlignment = Alignment.Center) {
            Icon(Icons.Filled.ExpandLess, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(12.dp))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TodoBottomSheet(items: List<TodoItem>, completed: Int, total: Int, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Text("Tasks ($completed/$total)", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                items.forEach { item ->
                    val done = item.status == "completed" || item.status == "done" || item.status == "complete"
                    val inProgress = item.status == "in_progress"
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp).clip(RoundedCornerShape(12.dp))
                            .background(if (inProgress) Color(0xFF18181C) else if (done) Color(0xFF121214).copy(alpha = 0.6f) else Color(0xFF141417))
                            .border(1.dp, if (inProgress) Color(0xFF3F3F46) else if (done) Color(0xFF222226) else Color(0xFF27272A), RoundedCornerShape(12.dp))
                            .padding(12.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            modifier = Modifier.size(20.dp).clip(RoundedCornerShape(6.dp))
                                .background(if (done) Color(0xFF14532D).copy(alpha = 0.4f) else if (inProgress) Color(0xFF1E293B) else Color(0xFF18181B))
                                .border(1.dp, if (done) Color(0xFF22C55E) else if (inProgress) Color(0xFF38BDF8) else Color(0xFF3F3F46), RoundedCornerShape(6.dp)),
                            contentAlignment = Alignment.Center,
                        ) {
                            if (done) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF22C55E), modifier = Modifier.size(12.dp))
                            else if (inProgress) Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(Color(0xFF38BDF8)))
                        }
                        Text(
                            item.content, fontSize = 14.sp, lineHeight = 20.sp,
                            color = if (done) ConsoleColors.TextMuted else if (inProgress) Color(0xFFFAFAFA) else Color(0xFFD4D4D8),
                            fontWeight = if (inProgress) FontWeight.Medium else FontWeight.Normal,
                            textDecoration = if (done) TextDecoration.LineThrough else null,
                            modifier = Modifier.weight(1f).padding(start = 12.dp),
                        )
                        if (inProgress) {
                            Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(999.dp)).background(Color(0xFF0284C7).copy(alpha = 0.2f)).border(1.dp, Color(0xFF38BDF8).copy(alpha = 0.3f), RoundedCornerShape(999.dp)).padding(horizontal = 8.dp, vertical = 2.dp)) {
                                Text("In Progress", color = Color(0xFF38BDF8), fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
                            }
                        } else if (done) {
                            Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(999.dp)).background(Color(0xFF15803D).copy(alpha = 0.2f)).border(1.dp, Color(0xFF22C55E).copy(alpha = 0.3f), RoundedCornerShape(999.dp)).padding(horizontal = 8.dp, vertical = 2.dp)) {
                                Text("Done", color = Color(0xFF22C55E), fontSize = 10.sp, fontWeight = FontWeight.SemiBold)
                            }
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SubagentSheet(subagents: List<SubagentInfo>, selectedId: String?, onSelect: (String?) -> Unit, onDismiss: () -> Unit, onOpenDetails: (String) -> Unit) {
    var selected by remember(selectedId) { mutableStateOf(selectedId) }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Text("Subagents (${subagents.size})", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
            Column(modifier = Modifier.verticalScroll(rememberScrollState())) {
                subagents.forEach { s ->
                    val sel = s.subagentId == selected
                    Row(
                        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(RoundedCornerShape(12.dp))
                            .background(if (sel) ConsoleColors.CardAlt else ConsoleColors.Card)
                            .border(1.dp, if (sel) ConsoleColors.Border else ConsoleColors.BorderSubtle, RoundedCornerShape(12.dp))
                            .clickable {
                                selected = s.subagentId
                                onSelect(s.subagentId)
                                onOpenDetails(s.subagentId)
                            }
                            .padding(14.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(if (s.status == "running") Color(0xFF38BDF8) else if (s.status == "completed") Color(0xFF34D399) else Color(0xFFF87171)))
                        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
                            Text(s.name.ifBlank { s.role.ifBlank { "Subagent" } }, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                            Text("${s.role} · ${s.status}", color = ConsoleColors.TextSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
                        }
                    }
                }
            }
        }
    }
}
