package com.console.mobile.feature.chat

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.ScrollState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.CompositingStrategy
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.AppContainer
import com.console.mobile.core.util.ChangeTotals
import com.console.mobile.core.util.formatCompactCount
import com.console.mobile.core.util.splitRing
import com.console.mobile.data.model.Automation
import com.console.mobile.data.model.BackgroundWork
import com.console.mobile.data.model.BackgroundWorkOutput
import com.console.mobile.data.model.WebhookSubscription
import com.console.mobile.data.model.applyFrame
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowLeft
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowUp
import io.github.lyxnx.compose.ui.tablericons.outline.Bolt
import io.github.lyxnx.compose.ui.tablericons.outline.Calendar
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerPause
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerPlay
import io.github.lyxnx.compose.ui.tablericons.outline.X
import io.github.lyxnx.compose.ui.tablericons.outline.FileDiff
import io.github.lyxnx.compose.ui.tablericons.outline.GridDots
import io.github.lyxnx.compose.ui.tablericons.outline.ListCheck
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerStop
import io.github.lyxnx.compose.ui.tablericons.outline.Robot
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2
import console.v1.SubagentInfo
import console.v1.TodoItem
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.OptionRow
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TintPill
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme

fun todoCounts(items: List<TodoItem>): Pair<Int, Int> {
    val done = items.count { it.status == "completed" || it.status == "done" || it.status == "complete" }
    return done to items.size
}

/**
 * The instant worktree launch's state banner, above the composer:
 * "Creating worktree…" while the server cuts the worktree (the prompt
 * auto-runs the moment it exists), and a persistent error with Retry and
 * Discard once the launch has failed.
 */
@Composable
fun WorktreeLaunchBanner(
    state: String?,
    error: String?,
    onRetry: () -> Unit,
    onDiscard: () -> Unit,
) {
    when (state) {
        "provisioning" -> Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 2.dp)
                .clip(RoundedCornerShape(8.dp))
                .background(NewTheme.Raised)
                .padding(horizontal = 10.dp, vertical = 7.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(modifier = Modifier.size(8.dp).clip(CircleShape).background(NewTheme.Accent))
            Spacer(modifier = Modifier.size(8.dp))
            Text("Creating worktree…", color = NewTheme.TextSecondary, fontSize = 12.sp)
        }
        "failed" -> Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(vertical = 2.dp)
                .clip(RoundedCornerShape(8.dp))
                .background(NewTheme.Raised)
                .border(1.dp, NewTheme.Danger.copy(alpha = 0.35f), RoundedCornerShape(8.dp))
                .padding(horizontal = 10.dp, vertical = 7.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                error?.takeIf { it.isNotBlank() } ?: "Worktree launch failed",
                color = NewTheme.Danger,
                fontSize = 12.sp,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Text(
                "Retry",
                color = NewTheme.TextPrimary,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable { onRetry() }
                    .padding(horizontal = 8.dp, vertical = 4.dp),
            )
            Text(
                "Discard",
                color = NewTheme.TextSecondary,
                fontSize = 12.sp,
                fontWeight = FontWeight.Medium,
                modifier = Modifier
                    .clip(RoundedCornerShape(6.dp))
                    .clickable { onDiscard() }
                    .padding(horizontal = 8.dp, vertical = 4.dp),
            )
        }
    }
}

/**
 * The counts floating above the composer: one compact pill each for tasks,
 * subagents and shells. Tasks and shells open their sheet; subagents toggle
 * the floating panel above the pill. The icon takes the accent while that
 * kind of work is still running.
 */
@Composable
fun ComposerPills(
    completedTasks: Int,
    totalTasks: Int,
    showTasks: Boolean,
    subagents: List<SubagentInfo>,
    shellCount: Int,
    automationCount: Int = 0,
    webhookCount: Int = 0,
    hasActiveAutomationOrWebhook: Boolean = false,
    changes: ChangeTotals,
    onTasks: () -> Unit,
    onSubagents: () -> Unit,
    onShells: () -> Unit,
    onAutomations: () -> Unit = {},
    onChanges: () -> Unit,
    suggestions: List<String> = emptyList(),
    onSuggestion: (String) -> Unit = {},
    onEditSuggestion: (String) -> Unit = {},
    trailing: (@Composable () -> Unit)? = null,
) {
    if (!showTasks && subagents.isEmpty() && shellCount == 0 && automationCount == 0 && webhookCount == 0 && changes.files == 0 && suggestions.isEmpty() && trailing == null) return
    val scroll = rememberScrollState()
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 10.dp).padding(bottom = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier.weight(1f).fadeScrollEdges(scroll).horizontalScroll(scroll),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (changes.files > 0) {
                ChangesPill(changes = changes, onClick = onChanges)
            }
            if (showTasks) {
                ContextPill(
                    icon = TablerIcons.Outline.ListCheck,
                    text = "$completedTasks/$totalTasks tasks",
                    running = false,
                    onClick = onTasks,
                )
            }
            if (subagents.isNotEmpty()) {
                ContextPill(
                    icon = TablerIcons.Outline.GridDots,
                    text = "${subagents.size} ${if (subagents.size == 1) "subagent" else "subagents"}",
                    running = subagents.any { it.status == "running" },
                    onClick = onSubagents,
                )
            }
            if (shellCount > 0) {
                ContextPill(
                    icon = TablerIcons.Outline.Terminal2,
                    text = "$shellCount ${if (shellCount == 1) "shell" else "shells"}",
                    running = true,
                    onClick = onShells,
                )
            }
            if (automationCount > 0 || webhookCount > 0) {
                val label = when {
                    automationCount > 0 && webhookCount > 0 -> "$automationCount sched · $webhookCount hook"
                    automationCount > 0 -> "$automationCount ${if (automationCount == 1) "schedule" else "schedules"}"
                    else -> "$webhookCount ${if (webhookCount == 1) "webhook" else "webhooks"}"
                }
                ContextPill(
                    icon = if (automationCount > 0) TablerIcons.Outline.Calendar else TablerIcons.Outline.Bolt,
                    text = label,
                    running = hasActiveAutomationOrWebhook,
                    onClick = onAutomations,
                )
            }
            // Follow-ups share the scroller, after the status pills, so a phone
            // keeps one row and the pills never lose their place.
            suggestions.forEach { text ->
                SuggestionPill(
                    text = text,
                    onClick = { onSuggestion(text) },
                    onLongClick = { onEditSuggestion(text) },
                )
            }
        }
        // Sits on the same level as the pills, pinned to the right.
        trailing?.invoke()
    }
}

/**
 * Session change totals: a ring split by lines added and removed, then the
 * compact "+4.3k -1.6k". The file count rides in the accessibility label.
 * Tapping opens the Changes tab.
 */
@Composable
private fun ChangesPill(changes: ChangeTotals, onClick: () -> Unit) {
    val label = "${changes.files} ${if (changes.files == 1) "file" else "files"} changed, +${changes.additions} -${changes.deletions}"
    val spans = splitRing(listOf(changes.additions.toFloat(), changes.deletions.toFloat()), gap = 0.06f, min = 0.06f)
    val addColor = NewTheme.Success
    val delColor = NewTheme.Danger
    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(999.dp))
            .background(NewTheme.Raised)
            .clickable(onClick = onClick)
            .semantics { contentDescription = label }
            .padding(start = 11.dp, end = 13.dp, top = 9.dp, bottom = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Canvas(modifier = Modifier.size(16.dp)) {
            val stroke = 2.dp.toPx()
            val inset = stroke / 2
            val arcSize = Size(size.width - stroke, size.height - stroke)
            listOf(addColor, delColor).zip(spans).forEach { (color, span) ->
                if (span.len > 0f) {
                    drawArc(color, -90f + 360f * span.start, 360f * span.len, false, Offset(inset, inset), arcSize, style = Stroke(stroke))
                }
            }
        }
        if (changes.additions > 0) {
            Text(
                text = "+${formatCompactCount(changes.additions)}",
                color = addColor,
                fontSize = 13.sp,
                fontFamily = ConsoleMonoFamily,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                modifier = Modifier.padding(start = 8.dp),
            )
        }
        if (changes.deletions > 0) {
            Text(
                text = "-${formatCompactCount(changes.deletions)}",
                color = delColor,
                fontSize = 13.sp,
                fontFamily = ConsoleMonoFamily,
                fontWeight = FontWeight.Medium,
                maxLines = 1,
                modifier = Modifier.padding(start = 6.dp),
            )
        }
    }
}

/**
 * Softens the edge of a horizontal scroller that still has content beyond it,
 * so a pill ends in a fade and not a hard slice. An edge with nothing further
 * to scroll stays crisp.
 */
private fun Modifier.fadeScrollEdges(scroll: ScrollState, width: Dp = 28.dp): Modifier = this
    .graphicsLayer { compositingStrategy = CompositingStrategy.Offscreen }
    .drawWithContent {
        drawContent()
        val fade = width.toPx()
        if (scroll.value > 0) {
            drawRect(
                brush = Brush.horizontalGradient(listOf(Color.Transparent, Color.Black), startX = 0f, endX = fade),
                size = Size(fade, size.height),
                blendMode = BlendMode.DstIn,
            )
        }
        if (scroll.value < scroll.maxValue) {
            drawRect(
                brush = Brush.horizontalGradient(listOf(Color.Black, Color.Transparent), startX = size.width - fade, endX = size.width),
                topLeft = Offset(size.width - fade, 0f),
                size = Size(fade, size.height),
                blendMode = BlendMode.DstIn,
            )
        }
    }

/**
 * One follow-up prompt. The pill shape of its neighbours, with an accent send
 * button so it reads as tappable rather than informational. The background is
 * the solid [NewTheme.Raised]: a translucent press tint would let the
 * transcript show through.
 */
@Composable
private fun SuggestionPill(text: String, onClick: () -> Unit, onLongClick: () -> Unit) {
    var menuOpen by remember { mutableStateOf(false) }
    Box {
    Row(
        modifier = Modifier
            .widthIn(max = 260.dp)
            .clip(RoundedCornerShape(999.dp))
            .background(NewTheme.Raised)
            .combinedClickable(onClick = onClick, onLongClick = { menuOpen = true })
            .padding(start = 5.dp, end = 13.dp, top = 5.dp, bottom = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier = Modifier.size(24.dp).clip(CircleShape).background(NewTheme.Accent),
            contentAlignment = Alignment.Center,
        ) {
            Icon(TablerIcons.Outline.ArrowUp, contentDescription = "Send", tint = Color.White, modifier = Modifier.size(14.dp))
        }
        Text(
            text = text,
            color = NewTheme.TextSecondary,
            fontSize = 13.sp,
            fontWeight = FontWeight.Medium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(start = 8.dp),
        )
    }
    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
        DropdownMenuItem(
            text = { Text("Send", color = NewTheme.TextPrimary) },
            onClick = { menuOpen = false; onClick() },
            leadingIcon = { Icon(TablerIcons.Outline.ArrowUp, contentDescription = null, tint = NewTheme.TextMuted) },
        )
        DropdownMenuItem(
            text = { Text("Edit Before Sending", color = NewTheme.TextPrimary) },
            onClick = { menuOpen = false; onLongClick() },
            leadingIcon = { Icon(TablerIcons.Outline.Edit, contentDescription = null, tint = NewTheme.TextMuted) },
        )
    }
    }
}

/** A small floating pill: an icon and a count, the way context above the composer is summarised. */
@Composable
private fun ContextPill(icon: ImageVector, text: String, running: Boolean, onClick: () -> Unit) {
    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(999.dp))
            .background(NewTheme.Raised)
            .clickable(onClick = onClick)
            .padding(horizontal = 12.dp, vertical = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            icon,
            contentDescription = null,
            tint = if (running) NewTheme.Accent else NewTheme.TextMuted,
            modifier = Modifier.size(15.dp),
        )
        Text(
            text, color = NewTheme.TextSecondary, fontSize = 13.sp, fontWeight = FontWeight.Medium,
            maxLines = 1, modifier = Modifier.padding(start = 7.dp),
        )
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

/**
 * The floating list that opens above the subagent pill: one row per subagent,
 * with its name and how long it has taken. The panel hugs its rows rather than
 * stretching across the screen, and sits above the composer instead of
 * covering the transcript in a sheet.
 */
@Composable
fun SubagentFloatingPanel(
    subagents: List<SubagentInfo>,
    onOpenDetails: (String) -> Unit,
) {
    // Running rows count up while the panel is open; a completed row keeps the
    // duration it finished with.
    val anyRunning = subagents.any { it.status == "running" }
    var now by remember { mutableStateOf(System.currentTimeMillis()) }
    LaunchedEffect(anyRunning) {
        while (anyRunning) {
            now = System.currentTimeMillis()
            delay(1_000)
        }
    }
    Column(
        modifier = Modifier
            .padding(horizontal = 10.dp)
            .padding(bottom = 8.dp)
            .widthIn(max = 280.dp)
            .clip(RoundedCornerShape(20.dp))
            .background(NewTheme.Card),
    ) {
        Column(modifier = Modifier.heightIn(max = 320.dp).verticalScroll(rememberScrollState())) {
            subagents.forEachIndexed { index, subagent ->
                SubagentPanelRow(subagent = subagent, now = now, onClick = { onOpenDetails(subagent.subagent_id) })
                if (index < subagents.lastIndex) SectionDivider(startInset = 40.dp)
            }
        }
    }
}

@Composable
private fun SubagentPanelRow(subagent: SubagentInfo, now: Long, onClick: () -> Unit) {
    val tint = when (subagent.status) {
        "running" -> NewTheme.Accent
        "completed" -> NewTheme.Success
        else -> NewTheme.Danger
    }
    Row(
        modifier = Modifier.clickable(onClick = onClick).padding(horizontal = 14.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            TablerIcons.Outline.GridDots,
            contentDescription = null,
            tint = tint,
            modifier = Modifier.size(16.dp),
        )
        Text(
            subagent.name.ifBlank { subagent.role.ifBlank { "Subagent" } },
            color = NewTheme.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium,
            maxLines = 1, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(start = 10.dp).widthIn(max = 170.dp),
        )
        val duration = subagentDuration(subagent, now)
        if (duration.isNotEmpty()) {
            Text(
                duration, color = NewTheme.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily,
                modifier = Modifier.padding(start = 12.dp),
            )
        }
    }
}

/** Elapsed time for a subagent, in the compact form the list uses: "3m 01s", "1h 03m". */
private fun subagentDuration(subagent: SubagentInfo, now: Long): String {
    if (subagent.created_at <= 0) return ""
    val end = if (subagent.status == "running") now else subagent.updated_at.takeIf { it > 0 } ?: now
    val seconds = ((end - subagent.created_at) / 1000).coerceAtLeast(0)
    return when {
        seconds >= 3600 -> "${seconds / 3600}h ${"%02d".format((seconds % 3600) / 60)}m"
        seconds >= 60 -> "${seconds / 60}m ${"%02d".format(seconds % 60)}s"
        else -> "${seconds}s"
    }
}

private fun shellCommand(work: BackgroundWork?): String =
    work?.metadata?.get("command")?.lineSequence()?.firstOrNull()?.trim().orEmpty()
        .ifBlank { work?.summary?.trim().orEmpty() }
        .ifBlank { work?.let { "Shell ${it.id.take(6)}" }.orEmpty() }

@Composable
fun BackgroundShellSheet(
    sessionId: String,
    works: List<BackgroundWork>,
    onCancel: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var selected by remember { mutableStateOf<BackgroundWork?>(null) }
    val current = selected
    if (current == null) {
        BaseSheet(onDismiss = onDismiss, title = "Shells running (${works.size})") {
            if (works.isEmpty()) {
                SectionCard { Note("No background shells are running.") }
            } else {
                SectionCard {
                    works.forEachIndexed { index, work ->
                        OptionRow(
                            title = shellCommand(work),
                            subtitle = "running · tap to view output",
                            monoSubtitle = true,
                            leading = { Box(modifier = Modifier.size(9.dp).clip(CircleShape).background(NewTheme.Accent)) },
                            trailing = { StopShellButton(onClick = { onCancel(work.id) }) },
                        ) {
                            selected = work
                        }
                        if (index < works.lastIndex) SectionDivider(startInset = 42.dp)
                    }
                }
            }
        }
    } else {
        ShellOutputSheet(
            sessionId = sessionId,
            work = current,
            onStop = { onCancel(current.id) },
            onBack = { selected = null },
            onDismiss = onDismiss,
        )
    }
}

@Composable
private fun ShellOutputSheet(
    sessionId: String,
    work: BackgroundWork,
    onStop: () -> Unit,
    onBack: () -> Unit,
    onDismiss: () -> Unit,
) {
    var output by remember(work.id) { mutableStateOf<BackgroundWorkOutput?>(null) }
    var unavailable by remember(work.id) { mutableStateOf(false) }
    var stopping by remember(work.id) { mutableStateOf(false) }
    val scroll = rememberScrollState()

    // Live output is pushed by the server as the shell writes; the flow ends
    // when the shell stops running.
    LaunchedEffect(work.id) {
        try {
            AppContainer.chatRepository.backgroundWorkOutputStream(sessionId, work.id).collect { frame ->
                output = output.applyFrame(frame)
            }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            if (output == null) unavailable = true
        }
    }

    val current = output
    val running = current?.status?.let { it == "running" } ?: !unavailable
    val statusText = when {
        current == null && unavailable -> "Output unavailable"
        current == null -> "Connecting…"
        current.timedOut -> "Timed out"
        current.status == "running" -> "Running"
        current.status == "killed" -> "Stopped"
        current.status == "exited" -> "Exited${current.exitCode?.let { " · code $it" }.orEmpty()}"
        current.status == "failed" -> "Failed${current.exitCode?.let { " · code $it" }.orEmpty()}"
        else -> current.status
    }
    val stdout = current?.stdout.orEmpty()
    val stderr = current?.stderr.orEmpty()
    val text = buildString {
        append(stdout)
        if (stderr.isNotBlank()) {
            if (isNotEmpty() && !endsWith("\n")) append('\n')
            append(stderr)
        }
    }

    LaunchedEffect(stdout.length, stderr.length) {
        if (current == null) return@LaunchedEffect
        withFrameNanos { }
        scroll.animateScrollTo(scroll.maxValue)
    }

    BaseSheet(onDismiss = onDismiss, title = "Shell output", scrollable = false) {
        Row(modifier = Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
            Row(
                modifier = Modifier.clip(RoundedCornerShape(8.dp)).clickable(onClick = onBack).padding(horizontal = 4.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(TablerIcons.Outline.ArrowLeft, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(16.dp))
                Text("Shells", color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(start = 6.dp))
            }
            Spacer(modifier = Modifier.weight(1f))
            if (running) {
                StopShellButton(onClick = { if (!stopping) { stopping = true; onStop() } }, enabled = !stopping)
            }
        }
        Text(
            shellCommand(work),
            color = NewTheme.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.Medium,
            fontFamily = ConsoleMonoFamily, maxLines = 2, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(horizontal = 4.dp).padding(top = 6.dp, bottom = 6.dp),
        )
        Row(
            modifier = Modifier.padding(horizontal = 4.dp).padding(bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier.size(7.dp).clip(CircleShape)
                    .background(if (running) NewTheme.Accent else NewTheme.TextMuted),
            )
            Text(statusText, color = NewTheme.TextMuted, fontSize = 11.sp, modifier = Modifier.padding(start = 7.dp))
            if (current?.truncated == true) {
                Text("· output truncated", color = NewTheme.Danger, fontSize = 11.sp, modifier = Modifier.padding(start = 8.dp))
            }
        }
        Column(
            modifier = Modifier.fillMaxWidth().heightIn(max = 420.dp)
                .clip(RoundedCornerShape(NewTheme.FieldRadius))
                .background(NewTheme.Card)
                .verticalScroll(scroll)
                .padding(12.dp),
        ) {
            Text(
                text.ifBlank { if (unavailable) "No output available." else "Waiting for output…" },
                color = if (unavailable) NewTheme.Danger else NewTheme.TextSecondary,
                fontSize = 11.sp, lineHeight = 16.sp, fontFamily = ConsoleMonoFamily,
            )
        }
    }
}

@Composable
private fun StopShellButton(onClick: () -> Unit, enabled: Boolean = true) {
    Box(
        modifier = Modifier.size(36.dp).clip(RoundedCornerShape(999.dp))
            .background(NewTheme.Danger.copy(alpha = if (enabled) 0.16f else 0.08f))
            .clickable(enabled = enabled, onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            TablerIcons.Outline.PlayerStop,
            contentDescription = "Stop shell",
            tint = NewTheme.Danger.copy(alpha = if (enabled) 1f else 0.5f),
            modifier = Modifier.size(16.dp),
        )
    }
}

@Composable
fun AutomationsBottomSheet(
    automations: List<Automation>,
    webhooks: List<WebhookSubscription>,
    onPauseAutomation: (String) -> Unit,
    onResumeAutomation: (String) -> Unit,
    onDeleteAutomation: (String) -> Unit,
    onDeleteWebhook: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val total = automations.size + webhooks.size
    BaseSheet(onDismiss = onDismiss, title = "Automations & Webhooks ($total)") {
        if (total == 0) {
            SectionCard { Note("No active automations or webhooks on this chat.") }
        } else {
            SectionCard {
                automations.forEachIndexed { index, auto ->
                    val isPaused = auto.status == "paused"
                    val isError = auto.status == "error"
                    val subtitle = if (auto.kind == "interval") {
                        "Every ${auto.intervalSeconds}s · ${auto.status}"
                    } else {
                        "One-shot · ${auto.status}"
                    }
                    OptionRow(
                        title = auto.prompt,
                        subtitle = subtitle,
                        leading = {
                            Icon(
                                TablerIcons.Outline.Calendar,
                                contentDescription = null,
                                tint = if (isError) NewTheme.Danger else if (isPaused) NewTheme.TextMuted else NewTheme.Accent,
                                modifier = Modifier.size(18.dp),
                            )
                        },
                        trailing = {
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Box(
                                    modifier = Modifier.size(34.dp).clip(CircleShape)
                                        .background(NewTheme.Raised)
                                        .clickable {
                                            if (isPaused || isError) onResumeAutomation(auto.id)
                                            else onPauseAutomation(auto.id)
                                        },
                                    contentAlignment = Alignment.Center,
                                ) {
                                    Icon(
                                        if (isPaused || isError) TablerIcons.Outline.PlayerPlay else TablerIcons.Outline.PlayerPause,
                                        contentDescription = if (isPaused || isError) "Resume" else "Pause",
                                        tint = NewTheme.TextSecondary,
                                        modifier = Modifier.size(15.dp),
                                    )
                                }
                                Box(
                                    modifier = Modifier.size(34.dp).clip(CircleShape)
                                        .background(NewTheme.Danger.copy(alpha = 0.16f))
                                        .clickable { onDeleteAutomation(auto.id) },
                                    contentAlignment = Alignment.Center,
                                ) {
                                    Icon(
                                        TablerIcons.Outline.X,
                                        contentDescription = "Delete",
                                        tint = NewTheme.Danger,
                                        modifier = Modifier.size(15.dp),
                                    )
                                }
                            }
                        },
                    ) {}
                    if (index < automations.lastIndex || webhooks.isNotEmpty()) SectionDivider(startInset = 42.dp)
                }
                webhooks.forEachIndexed { index, hook ->
                    OptionRow(
                        title = hook.prompt,
                        subtitle = "Webhook · ${hook.topic} · ${hook.status}",
                        leading = {
                            Icon(
                                TablerIcons.Outline.Bolt,
                                contentDescription = null,
                                tint = NewTheme.Accent,
                                modifier = Modifier.size(18.dp),
                            )
                        },
                        trailing = {
                            Box(
                                modifier = Modifier.size(34.dp).clip(CircleShape)
                                    .background(NewTheme.Danger.copy(alpha = 0.16f))
                                    .clickable { onDeleteWebhook(hook.id) },
                                contentAlignment = Alignment.Center,
                            ) {
                                Icon(
                                    TablerIcons.Outline.X,
                                    contentDescription = "Delete",
                                    tint = NewTheme.Danger,
                                    modifier = Modifier.size(15.dp),
                                )
                            }
                        },
                    ) {}
                    if (index < webhooks.lastIndex) SectionDivider(startInset = 42.dp)
                }
            }
        }
    }
}
