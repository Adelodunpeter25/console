package com.console.mobile.feature.changes

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Checkbox
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.projectForSession
import com.console.mobile.core.util.ChangesRow
import com.console.mobile.core.util.baseOf
import com.console.mobile.core.util.buildRows
import com.console.mobile.core.util.parseUnifiedDiff
import com.console.mobile.core.util.statusColorHex
import com.console.mobile.core.util.statusLetter
import com.console.mobile.core.util.stripRepoPrefix
import com.console.mobile.core.util.sumTotals
import com.console.mobile.feature.chat.DiffSummaryBadge
import com.console.mobile.feature.chat.DiffView
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.CircleIconButton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.SectionSkeleton
import com.console.mobile.ui.theme.NewTheme
import androidx.compose.material3.CheckboxDefaults
import androidx.compose.foundation.layout.width
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.GitBranch

/**
 * Session file changes for the active chat: folder-grouped file list with
 * reviewed checkmarks → cached unified diff per file. Backed by
 * GET /api/sessions/:id/changes (+ diff + reviewed toggle).
 */
@Composable
fun ChangesScreen(onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val sessionChanges by AppContainer.sessionStateHolder.sessionChanges.collectAsStateWithLifecycle()

    val sessionId = appState.selectedSessionId
    val sessionCwd = sessionId?.let { sessionViews[it]?.sessionCwd }
    val project = projectForSession(projectState.projects, sessionId?.let { sessionViews[it]?.projectId }, sessionCwd)
        ?: projectState.projects.firstOrNull { it.id == appState.selectedProjectId } ?: projectState.projects.firstOrNull()
    val repoPath = sessionCwd ?: project?.path

    val files = sessionId?.let { sessionChanges[it].orEmpty() }.orEmpty()
    var loading by remember(sessionId) { mutableStateOf(true) }
    var error by remember(sessionId) { mutableStateOf<String?>(null) }
    var collapsed by remember(sessionId) { mutableStateOf(setOf<String>()) }
    var selectedPath by remember(sessionId) { mutableStateOf<String?>(null) }
    BackHandler(enabled = selectedPath != null) { selectedPath = null }
    var selectedTurn by remember(sessionId) { mutableStateOf(0) }
    var diffText by remember { mutableStateOf<String?>(null) }
    var diffLoading by remember { mutableStateOf(false) }
    var diffError by remember { mutableStateOf<String?>(null) }
    val diffCache = remember(sessionId) { mutableMapOf<String, String?>() }

    fun refresh() {
        val sid = sessionId ?: return
        loading = true
        error = null
        scope.launch {
            try {
                withContext(Dispatchers.IO) { AppContainer.sessionRepository.loadSessionChanges(sid) }
            } catch (e: Exception) {
                error = e.message ?: "Failed to load changes."
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(sessionId) {
        selectedPath = null
        diffText = null
        diffError = null
        if (sessionId != null) refresh() else loading = false
    }

    LaunchedEffect(selectedPath, sessionId) {
        val sp = selectedPath
        val sid = sessionId
        if (sp == null || sid == null) {
            diffText = null
            diffError = null
            diffLoading = false
            return@LaunchedEffect
        }
        val turn = files.firstOrNull { it.path == sp }?.turn_index ?: 0
        selectedTurn = turn
        if (diffCache.containsKey(sp)) {
            diffText = diffCache[sp]
            diffError = null
            diffLoading = false
            return@LaunchedEffect
        }
        diffLoading = true
        diffText = null
        diffError = null
        try {
            val d = withContext(Dispatchers.IO) { AppContainer.sessionRepository.loadChangeDiff(sid, sp, turn) }
            diffCache[sp] = d
            if (selectedPath == sp) diffText = d
        } catch (e: Exception) {
            if (selectedPath == sp) diffError = e.message ?: "Failed to load diff."
        } finally {
            if (selectedPath == sp) diffLoading = false
        }
    }

    val totals = sumTotals(files)
    val rows = buildRows(files, collapsed, repoPath)

    // File diff view.
    val sel = selectedPath
    if (sel != null) {
        val change = files.firstOrNull { it.path == sel }
        Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
            PageHeader(title = baseOf(sel), subtitle = stripRepoPrefix(sel, repoPath), onBack = { selectedPath = null })
            Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                if (change != null) {
                    SectionCard(modifier = Modifier.padding(bottom = 16.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth().padding(start = 18.dp, end = 10.dp, top = 6.dp, bottom = 6.dp)) {
                            Text(statusLetter(change.status), color = parseChangeColor(statusColorHex(change.status)), fontSize = 14.sp, fontWeight = FontWeight.Bold)
                            Text("+${change.additions}", color = NewTheme.Success, fontSize = 14.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.padding(start = 12.dp))
                            Text("-${change.deletions}", color = NewTheme.Danger, fontSize = 14.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.padding(start = 10.dp))
                            Box(modifier = Modifier.weight(1f))
                            Text("Reviewed", color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(end = 4.dp))
                            ReviewCheckbox(
                                checked = change.reviewed,
                                onCheckedChange = {
                                    sessionId?.let { sid ->
                                        AppContainer.sessionRepository.toggleChangeReviewed(sid, change.path, change.turn_index, !change.reviewed)
                                    }
                                },
                            )
                        }
                    }
                }
                when {
                    diffLoading -> LoadingState()
                    diffError != null -> Banner(diffError ?: "Failed to load diff.")
                    diffText != null -> DiffView(diff = parseUnifiedDiff(diffText ?: ""), filePath = sel)
                    else -> Note("No diff available for this file.")
                }
            }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(
            title = "Changes",
            subtitle = "${totals.files} files  +${totals.additions} -${totals.deletions}",
            onBack = onBack,
            actions = { CircleIconButton(TablerIcons.Outline.Refresh, "Refresh", onClick = ::refresh) },
        )
        when {
            sessionId == null -> EmptyView(title = "No active session", description = "Open a chat to see its file changes.", icon = TablerIcons.Outline.GitBranch, modifier = Modifier.fillMaxSize())
            loading -> Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) { SectionSkeleton(rows = 4) }
            error != null -> EmptyView(title = "Couldn't load changes", description = error ?: "Failed to load session changes.", icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
            rows.isEmpty() -> EmptyView(title = "No file changes yet", description = "Files this session touches will show up here.", icon = TablerIcons.Outline.GitBranch, modifier = Modifier.fillMaxSize())
            else -> Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp)) {
                // One card per folder: the folder is the heading, its files are the rows.
                // The rows arrive flat (Folder, File, File, Folder, …), so regroup them.
                var i = 0
                while (i < rows.size) {
                    val folder = rows[i] as? ChangesRow.Folder
                    if (folder == null) { i++; continue }
                    val files = mutableListOf<ChangesRow.File>()
                    var j = i + 1
                    while (j < rows.size && rows[j] is ChangesRow.File) { files += rows[j] as ChangesRow.File; j++ }
                    val isCollapsed = collapsed.contains(folder.name)
                    FolderHeading(
                        name = folder.name, count = folder.count, collapsed = isCollapsed,
                        additions = folder.additions, deletions = folder.deletions,
                        onToggle = { collapsed = if (isCollapsed) collapsed - folder.name else collapsed + folder.name },
                    )
                    if (!isCollapsed && files.isNotEmpty()) {
                        SectionCard {
                            files.forEachIndexed { index, row ->
                                FileChangeRow(
                                    row = row,
                                    onOpen = { selectedPath = row.path },
                                    onToggleReviewed = {
                                        sessionId?.let { sid ->
                                            AppContainer.sessionRepository.toggleChangeReviewed(sid, row.path, row.turnIndex, !row.reviewed)
                                        }
                                    },
                                )
                                if (index < files.lastIndex) SectionDivider(startInset = 56.dp)
                            }
                        }
                    }
                    i = j
                }
            }
        }
    }
}

/** A folder's heading above its card: chevron, accent name and count, and its +/- totals. */
@Composable
private fun FolderHeading(name: String, count: Int, collapsed: Boolean, additions: Int, deletions: Int, onToggle: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onToggle).padding(start = 8.dp, end = 12.dp, top = 22.dp, bottom = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(if (collapsed) TablerIcons.Outline.ChevronRight else TablerIcons.Outline.ChevronUp, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(16.dp))
        Text("$name · $count", color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 6.dp))
        DiffSummaryBadge(addedCount = additions, removedCount = deletions)
    }
}

@Composable
private fun FileChangeRow(row: ChangesRow.File, onOpen: () -> Unit, onToggleReviewed: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(start = 18.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(statusLetter(row.status), color = parseChangeColor(statusColorHex(row.status)), fontSize = 13.sp, fontWeight = FontWeight.Bold, modifier = Modifier.width(14.dp))
        Box(modifier = Modifier.padding(start = 6.dp)) { FileIcon(filename = row.name, sizeDp = 20) }
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
            Text(row.name, color = NewTheme.TextPrimary, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(row.rel, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        DiffSummaryBadge(addedCount = row.additions, removedCount = row.deletions)
        ReviewCheckbox(checked = row.reviewed, onCheckedChange = onToggleReviewed)
    }
}

/** The "reviewed" tick, in the accent. */
@Composable
private fun ReviewCheckbox(checked: Boolean, onCheckedChange: () -> Unit) {
    Checkbox(
        checked = checked,
        onCheckedChange = { onCheckedChange() },
        colors = CheckboxDefaults.colors(
            checkedColor = NewTheme.Accent,
            uncheckedColor = NewTheme.TextMuted,
            checkmarkColor = NewTheme.OnPrimary,
        ),
    )
}

private fun ChangesRow.keyOf(): String = when (this) {
    is ChangesRow.Folder -> key
    is ChangesRow.File -> key
}

private fun parseChangeColor(hex: String): Color {
    return try {
        Color(android.graphics.Color.parseColor(hex))
    } catch (_: Exception) {
        NewTheme.TextMuted
    }
}
