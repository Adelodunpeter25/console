package com.console.mobile.feature.chat

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.ActivityEvent
import com.console.mobile.core.chat.RunActivityState
import com.console.mobile.core.chat.RunStatus
import com.console.mobile.core.util.formatDurationMs
import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import com.console.mobile.ui.theme.ConsoleColors
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
                CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(12.dp))
            }
            Text(summary, color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = if (isWorking) 6.dp else 0.dp))
            Icon(if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
        }
        if (expanded) {
            val groups = groupEvents(activity.events)
            groups.forEach { g ->
                when (g) {
                    is RenderGroup.Thinking -> CollapsibleThinking(text = g.text)
                    is RenderGroup.Text -> MarkdownText(content = g.text, modifier = Modifier.padding(start = 4.dp, bottom = 8.dp))
                    is RenderGroup.Tools -> {
                        val byId = g.results.associateBy { it.toolCallId }
                        g.calls.forEach { call ->
                            ToolCallRow(call = call, result = byId[call.id], cwd = cwd)
                        }
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
            Icon(Icons.Filled.AutoAwesome, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(12.dp))
            Text("Thought", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
            Icon(if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(13.dp))
        }
        if (expanded && text.isNotEmpty()) {
            Text(text, color = ConsoleColors.TextSecondary, fontSize = 13.sp, lineHeight = 20.sp, modifier = Modifier.padding(start = 18.dp))
        }
    }
}
