package com.console.mobile.feature.subagents

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.model.SubagentActivityItem
import com.console.mobile.data.model.SubagentInfo
import com.console.mobile.feature.chat.MarkdownText
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * Port of screens/subagents/subagents-screen.tsx + subagent-details-screen.tsx.
 */
@Composable
fun SubagentsScreen(onBackToChat: () -> Unit, onOpenDetails: (String) -> Unit) {
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val sessionId = appState.selectedSessionId

    LaunchedEffect(sessionId) {
        if (sessionId != null) AppContainer.chatRepository.loadSubagents(sessionId)
    }
    val subagents = sessionId?.let { chatSessions[it]?.subagents } ?: emptyList()

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(title = "Subagents (${subagents.size})", onBack = onBackToChat)
        if (subagents.isEmpty()) {
            EmptyState(
                title = "No Subagents Spawned",
                description = "Subagents created by the assistant during this session will stream their activity and summaries here in real time.",
                icon = {
                    Box(modifier = Modifier.size(48.dp).clip(CircleShape).background(Color(0xFF18181C)).border(1.dp, Color(0xFF27272A), CircleShape), contentAlignment = Alignment.Center) {
                        Icon(Icons.Filled.SmartToy, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(24.dp))
                    }
                },
            )
        } else {
            Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                subagents.forEach { s ->
                    SubagentCard(subagent = s, onClick = {
                        AppContainer.appStateHolder.setSelectedSubagentId(s.subagentId)
                        onOpenDetails(s.subagentId)
                    })
                }
            }
        }
    }
}

@Composable
private fun SubagentCard(subagent: SubagentInfo, onClick: () -> Unit) {
    val running = subagent.status == "running"
    val completed = subagent.status == "completed"
    val color = if (running) Color(0xFF38BDF8) else if (completed) Color(0xFF22C55E) else Color(0xFFEF4444)
    val label = if (running) "Running" else if (completed) "Done" else if (subagent.status == "aborted") "Aborted" else "Failed"
    val shape = RoundedCornerShape(12.dp)
    Column(
        modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp).clip(shape)
            .background(Color(0xFF141417))
            .border(1.dp, Color(0xFF27272A), shape)
            .clickable(onClick = onClick)
            .padding(14.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(modifier = Modifier.size(24.dp).clip(RoundedCornerShape(6.dp)).background(Color(0xFF1C1C20)).border(1.dp, Color(0xFF303036), RoundedCornerShape(6.dp)), contentAlignment = Alignment.Center) {
                Icon(Icons.Filled.SmartToy, contentDescription = null, tint = color, modifier = Modifier.size(13.dp))
            }
            Text(subagent.role.ifBlank { subagent.name.ifBlank { "Subagent" } }, color = Color(0xFFFAFAFA), fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
            Box(modifier = Modifier.clip(RoundedCornerShape(999.dp)).background(color.copy(alpha = 0.12f)).border(1.dp, color.copy(alpha = 0.4f), RoundedCornerShape(999.dp)).padding(horizontal = 10.dp, vertical = 2.dp)) {
                Text(label, color = color, fontSize = 10.5.sp, fontWeight = FontWeight.SemiBold)
            }
            Icon(Icons.Filled.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
        }
        if (subagent.prompt.isNotBlank()) {
            Text(subagent.prompt, color = ConsoleColors.TextSecondary, fontSize = 12.sp, lineHeight = 17.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 10.dp))
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("${subagent.activities.size} tool action${if (subagent.activities.size == 1) "" else "s"}", color = ConsoleColors.TextMuted, fontSize = 11.sp)
            Box(modifier = Modifier.weight(1f))
            Text("View details →", color = Color(0xFF38BDF8), fontSize = 11.sp, fontWeight = FontWeight.Medium)
        }
    }
}

@Composable
fun SubagentDetailsScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    var copied by remember { mutableStateOf(false) }
    val subagents = appState.selectedSessionId?.let { chatSessions[it]?.subagents } ?: emptyList()
    val subagent = subagents.firstOrNull { it.subagentId == appState.selectedSubagentId }

    val running = subagent?.status == "running"
    val completed = subagent?.status == "completed"
    val color = if (running) Color(0xFF38BDF8) else if (completed) Color(0xFF22C55E) else Color(0xFFEF4444)

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(title = subagent?.role ?: "Subagent Details", onBack = onBack)
        if (subagent == null) {
            EmptyState(title = "Subagent not found", description = "It may have been cleared when switching environments.")
            return@Column
        }
        Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
            // Status header.
            Row(
                modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Color(0xFF141417)).border(1.dp, Color(0xFF27272A), RoundedCornerShape(12.dp)).padding(14.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(color))
                Text(subagent.status.replaceFirstChar { it.uppercaseChar() }, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 10.dp))
                Text("Turn ${maxOf(1, subagent.currentTurn)}/${subagent.maxTurns}", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily)
            }
            // Prompt.
            Text("Prompt", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 20.dp, bottom = 8.dp))
            Box(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(12.dp)).padding(14.dp)) {
                Text(subagent.prompt, color = ConsoleColors.TextPrimary, fontSize = 13.sp, lineHeight = 20.sp)
            }
            // Summary (markdown) + copy.
            if (!subagent.summary.isNullOrBlank()) {
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 20.dp, bottom = 8.dp)) {
                    Text("Summary", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                    IconButton(onClick = {
                        try {
                            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                            cm.setPrimaryClip(ClipData.newPlainText("console", subagent.summary))
                            copied = true
                        } catch (_: Exception) {}
                    }, modifier = Modifier.size(28.dp)) {
                        if (copied) Icon(Icons.Filled.Check, contentDescription = "Copied", tint = Color(0xFF34D399), modifier = Modifier.size(14.dp))
                        else Icon(Icons.Filled.ContentCopy, contentDescription = "Copy", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(14.dp))
                    }
                }
                Box(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(12.dp)).padding(14.dp)) {
                    MarkdownText(content = subagent.summary ?: "")
                }
            }
            // Error.
            if (!subagent.error.isNullOrBlank()) {
                Row(modifier = Modifier.fillMaxWidth().padding(top = 12.dp).clip(RoundedCornerShape(12.dp)).background(Color(0xFFF87171).copy(alpha = 0.08f)).border(1.dp, Color(0xFFF87171).copy(alpha = 0.3f), RoundedCornerShape(12.dp)).padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Warning, contentDescription = null, tint = Color(0xFFF87171), modifier = Modifier.size(14.dp))
                    Text(subagent.error ?: "", color = Color(0xFFF87171), fontSize = 13.sp, modifier = Modifier.padding(start = 10.dp))
                }
            }
            // Activity groups.
            if (subagent.activities.isNotEmpty()) {
                Text("Activity (${subagent.activities.size})", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 20.dp, bottom = 8.dp))
                groupActivities(subagent.activities).forEach { g ->
                    ActivityGroupCard(group = g)
                }
            }
        }
    }
}

private data class ActivityGroup(val toolName: String, val activities: List<SubagentActivityItem>)

private fun groupActivities(activities: List<SubagentActivityItem>): List<ActivityGroup> {
    val out = mutableListOf<ActivityGroup>()
    for (a in activities) {
        val last = out.lastOrNull()
        if (last != null && last.toolName == a.toolName) {
            out[out.lastIndex] = last.copy(activities = last.activities + a)
        } else {
            out.add(ActivityGroup(a.toolName, listOf(a)))
        }
    }
    return out
}

@Composable
private fun ActivityGroupCard(group: ActivityGroup) {
    var open by remember(group.toolName, group.activities.size) { mutableStateOf(false) }
    val hasError = group.activities.any { it.status != "completed" && it.status != "running" }
    val isRunning = group.activities.any { it.status == "running" }
    val done = group.activities.count { it.status == "completed" }
    val shape = RoundedCornerShape(12.dp)
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(shape).background(Color(0xFF141417)).border(1.dp, Color(0xFF27272A), shape)) {
        Row(modifier = Modifier.fillMaxWidth().clickable { open = !open }.padding(10.dp), verticalAlignment = Alignment.CenterVertically) {
            if (isRunning) Icon(Icons.Filled.SmartToy, contentDescription = null, tint = Color(0xFF38BDF8), modifier = Modifier.size(13.dp))
            else if (hasError) Icon(Icons.Filled.Warning, contentDescription = null, tint = Color(0xFFEF4444), modifier = Modifier.size(13.dp))
            else Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF22C55E), modifier = Modifier.size(13.dp))
            Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(6.dp)).background(Color(0xFF222226)).border(1.dp, Color(0xFF33333A), RoundedCornerShape(6.dp)).padding(horizontal = 6.dp, vertical = 2.dp)) {
                Text(group.toolName, color = Color(0xFFFAFAFA), fontSize = 10.5.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.Medium)
            }
            Text("${group.activities.size} ${if (group.activities.size == 1) "call" else "calls"}", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.weight(1f).padding(start = 8.dp))
            Text(if (isRunning) "Running" else "$done/${group.activities.size}", color = ConsoleColors.TextMuted, fontSize = 10.sp)
            Icon(if (open) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(13.dp))
        }
        if (open) {
            Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp).padding(bottom = 8.dp)) {
                group.activities.forEach { a -> ActivityRowItem(activity = a) }
            }
        }
    }
}

@Composable
private fun ActivityRowItem(activity: SubagentActivityItem) {
    val summary = activity.summary ?: argSummaryOf(activity.args)
    val running = activity.status == "running"
    val done = activity.status == "completed"
    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        if (running) Icon(Icons.Filled.SmartToy, contentDescription = null, tint = Color(0xFF38BDF8), modifier = Modifier.size(13.dp))
        else if (done) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF22C55E), modifier = Modifier.size(13.dp))
        else Icon(Icons.Filled.Warning, contentDescription = null, tint = Color(0xFFEF4444), modifier = Modifier.size(13.dp))
        Box(modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(6.dp)).background(Color(0xFF222226)).border(1.dp, Color(0xFF33333A), RoundedCornerShape(6.dp)).padding(horizontal = 6.dp, vertical = 2.dp)) {
            Text(activity.toolName, color = Color(0xFFFAFAFA), fontSize = 10.5.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.Medium)
        }
        if (summary != null) {
            Text(summary, color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
        } else {
            androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
        }
        if (activity.error != null) {
            Text("Error", color = Color(0xFFEF4444), fontSize = 10.sp, modifier = Modifier.padding(start = 6.dp))
        }
    }
}

private fun argSummaryOf(args: kotlinx.serialization.json.JsonElement?): String? {
    val o = args as? JsonObject ?: return null
    fun s(key: String): String? = (o[key] as? JsonPrimitive)?.takeIf { it.isString }?.content
    val direct = s("command") ?: s("CommandLine") ?: s("path") ?: s("AbsolutePath") ?: s("SearchDirectory")
        ?: s("TargetFile") ?: s("pattern") ?: s("Pattern") ?: s("Query") ?: s("query") ?: s("url") ?: s("Url")
        ?: s("question") ?: s("directory") ?: s("SearchPath") ?: s("Prompt") ?: s("prompt") ?: s("filePath")
        ?: s("targetFile") ?: s("absolutePath")
    if (direct != null) return if (direct.length > 60) direct.take(57) + "…" else direct
    val firstKey = o.keys.firstOrNull() ?: return null
    val v = o[firstKey].toString()
    return "$firstKey: ${v.take(40)}"
}
