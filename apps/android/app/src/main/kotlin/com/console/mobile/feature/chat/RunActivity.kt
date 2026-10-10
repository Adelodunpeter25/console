package com.console.mobile.feature.chat
import com.console.mobile.feature.chat.markdown.CustomMarkdown

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
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
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Sparkles
import com.console.mobile.core.chat.ActivityEvent
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.util.formatDurationMs
import com.console.mobile.core.util.getToolIcon
import com.console.mobile.core.util.getToolLabel
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.delay

private sealed interface RenderGroup {
    data class Text(val id: String, val text: String) : RenderGroup
    data class Thinking(val id: String, val text: String) : RenderGroup
    data class Tools(val id: String, val calls: List<ToolCall>, val results: List<ToolResult>) : RenderGroup
}

private fun groupEvents(events: List<ActivityEvent>): List<RenderGroup> {
    val groups = mutableListOf<RenderGroup>()
    for (e in events) {
        when (e) {
            is ActivityEvent.Text -> groups.add(RenderGroup.Text(e.id, e.text))
            is ActivityEvent.Thinking -> groups.add(RenderGroup.Thinking(e.id, e.text))
            is ActivityEvent.ToolCallEvent -> {
                val last = groups.lastOrNull()
                if (last is RenderGroup.Tools && last.calls.isNotEmpty() && last.calls.last().name == e.call.name) {
                    val calls = last.calls + e.call
                    val results = if (e.result != null) last.results + e.result else last.results
                    groups[groups.lastIndex] = RenderGroup.Tools(last.id, calls, results)
                } else {
                    groups.add(RenderGroup.Tools(e.call.id, listOf(e.call), if (e.result != null) listOf(e.result) else emptyList()))
                }
            }
        }
    }
    return groups
}

/**
 * Port of components/chat/tools/run-activity.tsx.
 * Collapsible "Worked for Xs" header + grouped tool/thinking/text events.
 */
@Composable
fun RunActivity(activity: RunActivityState, running: Boolean, cwd: String? = null) {
    var expanded by remember(activity.runId, running) { mutableStateOf(running) }
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(running) {
        if (!running) return@LaunchedEffect
        while (true) {
            delay(1000)
            now = System.currentTimeMillis()
        }
    }
    val hasToolCalls = activity.events.any { it is ActivityEvent.ToolCallEvent }
    if (!hasToolCalls && !running) return

    val isWorking = running && activity.status == RunStatus.Working
    val elapsed = if (isWorking && activity.startedAt != null) now - activity.startedAt else activity.elapsedMs
    val summary = when {
        isWorking -> "Working for ${formatDurationMs(elapsed)}…"
        activity.status == RunStatus.Aborted -> "Aborted after ${formatDurationMs(elapsed)}"
        activity.status == RunStatus.Failed -> "Failed after ${formatDurationMs(elapsed)}"
        else -> "Worked for ${formatDurationMs(elapsed)}"
    }

    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)) {
        Row(
            modifier = Modifier.clickable { expanded = !expanded }.padding(vertical = 6.dp, horizontal = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (isWorking) {
                CircularProgressIndicator(color = NewTheme.Accent, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
            }
            Text(summary, color = NewTheme.TextSecondary, fontSize = 13.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = if (isWorking) 8.dp else 0.dp))
            // Collapsed points right (there is more to see), expanded points up.
            Icon(if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(14.dp))
        }
        // Always drawn, collapsed or not — one rule closing each run block.
        HorizontalDivider(color = NewTheme.Divider, thickness = 1.dp, modifier = Modifier.padding(horizontal = 4.dp).padding(vertical = 4.dp))
        if (expanded) {
            val groups = groupEvents(activity.events)
            groups.forEach { g ->
                when (g) {
                    is RenderGroup.Thinking -> CollapsibleThinking(text = g.text)
                    is RenderGroup.Text -> CustomMarkdown(content = g.text, modifier = Modifier.padding(start = 4.dp, bottom = 8.dp))
                    is RenderGroup.Tools -> {
                        val byId = g.results.associateBy { it.toolCallId }
                        if (g.calls.size == 1) {
                            val call = g.calls.first()
                            Box(modifier = Modifier.padding(vertical = 2.dp)) {
                                ToolCallRow(call = call, result = byId[call.id], cwd = cwd)
                            }
                        } else {
                            ToolGroupRow(
                                toolName = g.calls.first().name,
                                calls = g.calls,
                                results = g.results,
                                cwd = cwd,
                            )
                        }
                    }
                }
            }
        }
    }
}

@Composable
fun ToolGroupRow(
    toolName: String,
    calls: List<ToolCall>,
    results: List<ToolResult>,
    cwd: String? = null,
) {
    var expanded by remember(calls.firstOrNull()?.id) { mutableStateOf(false) }
    val byId = results.associateBy { it.toolCallId }
    val anyRunning = calls.any { byId[it.id] == null }
    val anyError = calls.any { byId[it.id]?.isError == true }
    val shape = RoundedCornerShape(NewTheme.FieldRadius)

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 3.dp)
            .clip(shape)
            .background(NewTheme.Card)
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable { expanded = !expanded }
                .padding(horizontal = 12.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                getToolIcon(toolName),
                contentDescription = null,
                tint = NewTheme.TextSecondary,
                modifier = Modifier.size(16.dp),
            )
            Spacer(Modifier.width(8.dp))
            Text(
                getToolLabel(toolName),
                color = NewTheme.TextSecondary,
                fontSize = 12.sp,
                fontWeight = FontWeight.SemiBold,
            )
            Text(
                "· ${calls.size} calls",
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                modifier = Modifier.padding(start = 6.dp),
            )
            Spacer(modifier = Modifier.weight(1f))
            // Desktop checks failure before pending (toolcalls.rs group_failed /
            // group_complete): an errored group shows the alert even if other
            // calls in it are still running.
            if (anyError) {
                Icon(
                    TablerIcons.Outline.AlertTriangle,
                    contentDescription = null,
                    tint = NewTheme.Danger,
                    modifier = Modifier.size(15.dp),
                )
            } else if (anyRunning) {
                CircularProgressIndicator(
                    color = NewTheme.TextMuted,
                    strokeWidth = 2.dp,
                    modifier = Modifier.size(15.dp),
                )
            } else {
                Icon(
                    TablerIcons.Outline.Check,
                    contentDescription = null,
                    tint = NewTheme.Success,
                    modifier = Modifier.size(15.dp),
                )
            }
            Spacer(Modifier.width(4.dp))
            Icon(
                if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown,
                contentDescription = null,
                tint = NewTheme.TextMuted,
                modifier = Modifier.size(15.dp),
            )
        }
        if (expanded) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 8.dp)
                    .padding(bottom = 8.dp)
            ) {
                calls.forEach { call ->
                    Box(modifier = Modifier.padding(vertical = 2.dp)) {
                        ToolCallRow(call = call, result = byId[call.id], cwd = cwd)
                    }
                }
            }
        }
    }
}

@Composable
private fun CollapsibleThinking(text: String) {
    var expanded by remember { mutableStateOf(false) }
    Column(modifier = Modifier.padding(bottom = 8.dp)) {
        Row(modifier = Modifier.clickable { expanded = !expanded }.padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(TablerIcons.Outline.Sparkles, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(14.dp))
            Text("Thought", color = NewTheme.TextSecondary, fontSize = 13.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 6.dp))
            Icon(if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(13.dp))
        }
        if (expanded && text.isNotEmpty()) {
            Text(text, color = NewTheme.TextSecondary, fontSize = 13.sp, lineHeight = 20.sp, modifier = Modifier.padding(start = 18.dp))
        }
    }
}
