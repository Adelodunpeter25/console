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
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Copy
import io.github.lyxnx.compose.ui.tablericons.outline.Robot
import com.console.mobile.AppContainer
import console.v1.SubagentActivityItem
import console.v1.SubagentInfo
import com.console.mobile.feature.chat.markdown.CustomMarkdown
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.data.api.ConsoleJson
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TintPill

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

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(title = "Subagents (${subagents.size})", onBack = onBackToChat)
        if (subagents.isEmpty()) {
            EmptyView(
                title = "No subagents spawned",
                description = "Subagents created by the assistant during this session will stream their activity and summaries here in real time.",
                icon = TablerIcons.Outline.Robot,
                modifier = Modifier.fillMaxSize(),
            )
        } else {
            Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                SectionCard(modifier = Modifier.padding(top = 8.dp)) {
                    subagents.forEachIndexed { index, s ->
                        SubagentRow(subagent = s, onClick = {
                            AppContainer.appStateHolder.setSelectedSubagentId(s.subagent_id)
                            onOpenDetails(s.subagent_id)
                        })
                        if (index < subagents.lastIndex) SectionDivider()
                    }
                }
            }
        }
    }
}

/** Colour a subagent's status is shown in: running = the accent, done = green, otherwise red. */
private fun subagentTint(status: String): Color = when (status) {
    "running" -> NewTheme.Accent
    "completed" -> NewTheme.Success
    else -> NewTheme.Danger
}

private fun subagentLabel(status: String): String = when (status) {
    "running" -> "Running"
    "completed" -> "Done"
    "aborted" -> "Aborted"
    else -> "Failed"
}

@Composable
private fun SubagentRow(subagent: SubagentInfo, onClick: () -> Unit) {
    val tint = subagentTint(subagent.status)
    Column(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 18.dp, vertical = 14.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(TablerIcons.Outline.Robot, contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
            Text(subagent.role.ifBlank { subagent.name.ifBlank { "Subagent" } }, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 14.dp))
            TintPill(subagentLabel(subagent.status), tint)
            Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(20.dp).padding(start = 4.dp))
        }
        if (subagent.prompt.isNotBlank()) {
            Text(subagent.prompt, color = NewTheme.TextSecondary, fontSize = 14.sp, lineHeight = 20.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 8.dp))
        }
        Text("${subagent.activities.size} tool action${if (subagent.activities.size == 1) "" else "s"}", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(top = 8.dp))
    }
}

@Composable
fun SubagentDetailsScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    var copied by remember { mutableStateOf(false) }
    val subagents = appState.selectedSessionId?.let { chatSessions[it]?.subagents } ?: emptyList()
    val subagent = subagents.firstOrNull { it.subagent_id == appState.selectedSubagentId }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(title = subagent?.role ?: "Subagent Details", onBack = onBack)
        if (subagent == null) {
            EmptyView(title = "Subagent not found", description = "It may have been cleared when switching environments.", icon = TablerIcons.Outline.Robot, modifier = Modifier.fillMaxSize())
            return@Column
        }
        val tint = subagentTint(subagent.status)
        Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 40.dp)) {
            // Status header.
            SectionCard(modifier = Modifier.padding(top = 8.dp)) {
                Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(modifier = Modifier.size(10.dp).clip(CircleShape).background(tint))
                    Text(subagent.status.replaceFirstChar { it.uppercaseChar() }, color = NewTheme.TextPrimary, fontSize = 17.sp, modifier = Modifier.weight(1f).padding(start = 14.dp))
                    Text("Turn ${maxOf(1, subagent.current_turn)}/${subagent.max_turns}", color = NewTheme.TextSecondary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily)
                }
            }
            // Prompt.
            Section("Prompt") {
                Text(subagent.prompt, color = NewTheme.TextPrimary, fontSize = 15.sp, lineHeight = 22.sp, modifier = Modifier.fillMaxWidth().padding(18.dp))
            }
            // Summary (markdown) + copy.
            if (!subagent.summary.isNullOrBlank()) {
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(start = 12.dp, top = 22.dp, bottom = 4.dp)) {
                    Text("Summary", color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium, modifier = Modifier.weight(1f))
                    IconButton(onClick = {
                        try {
                            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
                            cm.setPrimaryClip(ClipData.newPlainText("console", subagent.summary))
                            copied = true
                        } catch (_: Exception) {}
                    }, modifier = Modifier.size(36.dp)) {
                        if (copied) Icon(TablerIcons.Outline.Check, contentDescription = "Copied", tint = NewTheme.Success, modifier = Modifier.size(18.dp))
                        else Icon(TablerIcons.Outline.Copy, contentDescription = "Copy", tint = NewTheme.TextSecondary, modifier = Modifier.size(18.dp))
                    }
                }
                SectionCard { Box(modifier = Modifier.fillMaxWidth().padding(18.dp)) { CustomMarkdown(content = subagent.summary) } }
            }
            // Error.
            if (!subagent.error.isNullOrBlank()) {
                Banner(subagent.error, modifier = Modifier.padding(top = 16.dp))
            }
            // Activity groups.
            if (subagent.activities.isNotEmpty()) {
                Section("Activity (${subagent.activities.size})") {
                    val groups = groupActivities(subagent.activities)
                    groups.forEachIndexed { index, g ->
                        ActivityGroupRow(group = g)
                        if (index < groups.lastIndex) SectionDivider()
                    }
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
        if (last != null && last.toolName == a.tool_name) {
            out[out.lastIndex] = last.copy(activities = last.activities + a)
        } else {
            out.add(ActivityGroup(a.tool_name, listOf(a)))
        }
    }
    return out
}

@Composable
private fun ActivityGroupRow(group: ActivityGroup) {
    var open by remember(group.toolName, group.activities.size) { mutableStateOf(false) }
    val hasError = group.activities.any { it.status != "completed" && it.status != "running" }
    val isRunning = group.activities.any { it.status == "running" }
    val done = group.activities.count { it.status == "completed" }
    Column(modifier = Modifier.fillMaxWidth()) {
        Row(modifier = Modifier.fillMaxWidth().clickable { open = !open }.padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
            if (isRunning) Icon(TablerIcons.Outline.Robot, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(18.dp))
            else if (hasError) Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(18.dp))
            else Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(18.dp))
            ToolNameChip(group.toolName, Modifier.padding(start = 12.dp))
            Text("${group.activities.size} ${if (group.activities.size == 1) "call" else "calls"}", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.weight(1f).padding(start = 10.dp))
            Text(if (isRunning) "Running" else "$done/${group.activities.size}", color = NewTheme.TextMuted, fontSize = 12.sp)
            Icon(if (open) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(18.dp).padding(start = 4.dp))
        }
        if (open) {
            Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp).padding(bottom = 10.dp)) {
                group.activities.forEach { a -> ActivityRowItem(activity = a) }
            }
        }
    }
}

/** The tool's name in a small mono chip. */
@Composable
private fun ToolNameChip(name: String, modifier: Modifier = Modifier) {
    Text(
        name, color = NewTheme.TextPrimary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.Medium,
        modifier = modifier.clip(RoundedCornerShape(6.dp)).background(Color.White.copy(alpha = 0.08f)).padding(horizontal = 8.dp, vertical = 3.dp),
    )
}

@Composable
private fun ActivityRowItem(activity: SubagentActivityItem) {
    val summary = activity.summary ?: argSummaryOf(activity.args.toByteArray())
    val running = activity.status == "running"
    val done = activity.status == "completed"
    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        if (running) Icon(TablerIcons.Outline.Robot, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(16.dp))
        else if (done) Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.Success, modifier = Modifier.size(16.dp))
        else Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(16.dp))
        ToolNameChip(activity.tool_name, Modifier.padding(start = 10.dp))
        if (summary != null) {
            Text(summary, color = NewTheme.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 10.dp))
        } else {
            androidx.compose.foundation.layout.Spacer(modifier = Modifier.weight(1f))
        }
        if (activity.error != null) {
            Text("Error", color = NewTheme.Danger, fontSize = 12.sp, modifier = Modifier.padding(start = 6.dp))
        }
    }
}

private fun argSummaryOf(args: ByteArray): String? {
    if (args.isEmpty()) return null
    val o = try {
        ConsoleJson.parseToJsonElement(String(args, Charsets.UTF_8))
    } catch (_: Exception) {
        return null
    } as? JsonObject ?: return null
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
