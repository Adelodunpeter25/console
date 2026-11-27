package com.console.mobile.feature.changes

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.GitBranch
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import com.console.mobile.SharedNative
import com.console.mobile.core.chat.projectForSession
import com.console.mobile.core.util.ChangesRow
import com.console.mobile.core.util.buildRows
import com.console.mobile.core.util.changeSummary
import com.console.mobile.core.util.gitBadgeColor
import com.console.mobile.core.util.statusLetter
import com.console.mobile.feature.chat.DiffSummaryBadge
import com.console.mobile.feature.chat.DiffView
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.Note
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.SectionSkeleton
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Session file changes for the active chat: folder-grouped file list where
 * tapping a file opens its full unified diff right under the row. Backed by
 * GET /api/sessions/:id/changes (+ diff).
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
    // Every file starts closed; this holds the paths the user has opened.
    var expanded by remember(sessionId) { mutableStateOf(setOf<String>()) }
    val diffCache = remember(sessionId) { mutableMapOf<String, String>() }
    val listScroll = rememberScrollState()

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
        if (sessionId != null) refresh() else loading = false
    }

    val totals = changeSummary(files)
    val rows = buildRows(files, collapsed, repoPath)
    val allPaths = files.distinctBy { it.path }.map { it.path }

    Box(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        when {
            sessionId == null -> EmptyView(title = "No active session", description = "Open a chat to see its file changes.", icon = TablerIcons.Outline.GitBranch, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight))
            loading -> Column(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp).padding(top = PageHeaderHeight)) { SectionSkeleton(rows = 4) }
            error != null -> EmptyView(title = "Couldn't load changes", description = error ?: "Failed to load session changes.", icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight))
            rows.isEmpty() -> EmptyView(title = "No file changes yet", description = "Files this session touches will show up here.", icon = TablerIcons.Outline.GitBranch, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight))
            else -> Column(
                modifier = Modifier
                    .fillMaxSize()
                    .verticalScroll(listScroll)
                    .padding(horizontal = 16.dp)
                    .padding(top = PageHeaderHeight, bottom = 24.dp),
            ) {
                // One card per folder: the folder is the heading, its files are the rows.
                // The rows arrive flat (Folder, File, File, Folder, …), so regroup them.
                var i = 0
                while (i < rows.size) {
                    val folder = rows[i] as? ChangesRow.Folder
                    if (folder == null) { i++; continue }
                    val folderFiles = mutableListOf<ChangesRow.File>()
                    var j = i + 1
                    while (j < rows.size && rows[j] is ChangesRow.File) { folderFiles += rows[j] as ChangesRow.File; j++ }
                    val isCollapsed = collapsed.contains(folder.name)
                    FolderHeading(
                        name = folder.name, count = folder.count, collapsed = isCollapsed,
                        additions = folder.additions, deletions = folder.deletions,
                        onToggle = { collapsed = if (isCollapsed) collapsed - folder.name else collapsed + folder.name },
                    )
                    if (!isCollapsed && folderFiles.isNotEmpty()) {
                        SectionCard {
                            folderFiles.forEachIndexed { index, row ->
                                val isOpen = expanded.contains(row.path)
                                FileChangeRow(
                                    row = row,
                                    open = isOpen,
                                    onToggle = { expanded = if (isOpen) expanded - row.path else expanded + row.path },
                                )
                                if (isOpen) {
                                    val inline = files.firstOrNull { it.path == row.path && it.turn_index == row.turnIndex }?.diff_text
                                    FileDiffBody(sessionId = sessionId, row = row, inlineDiff = inline, cache = diffCache)
                                }
                                if (index < folderFiles.lastIndex) SectionDivider(startInset = 56.dp)
                            }
                        }
                    }
                    i = j
                }
            }
        }
        PageHeader(
            title = "Changes",
            subtitle = "${totals.files} files  +${totals.additions} -${totals.deletions}",
            blurred = true,
            onBack = onBack,
            actions = {
                if (rows.isNotEmpty()) {
                    val anyOpen = expanded.isNotEmpty()
                    HeaderIconButton(
                        icon = if (anyOpen) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown,
                        contentDescription = if (anyOpen) "Collapse all" else "Expand all",
                        onClick = { expanded = if (anyOpen) emptySet() else allPaths.toSet() },
                    )
                }
                HeaderIconButton(TablerIcons.Outline.Refresh, "Refresh", onClick = ::refresh)
            },
            modifier = Modifier.align(Alignment.TopCenter),
        )
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
private fun FileChangeRow(row: ChangesRow.File, open: Boolean, onToggle: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onToggle).padding(start = 18.dp, end = 12.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(statusLetter(row.status), color = gitBadgeColor(row.status), fontSize = 13.sp, fontWeight = FontWeight.Bold, modifier = Modifier.width(14.dp))
        Box(modifier = Modifier.padding(start = 6.dp)) { FileIcon(filename = row.name, sizeDp = 20) }
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
            Text(row.name, color = NewTheme.TextPrimary, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(row.rel, color = NewTheme.TextMuted, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        DiffSummaryBadge(addedCount = row.additions, removedCount = row.deletions)
        Icon(
            if (open) TablerIcons.Outline.ChevronDown else TablerIcons.Outline.ChevronRight,
            contentDescription = if (open) "Collapse" else "Expand",
            tint = NewTheme.TextMuted,
            modifier = Modifier.padding(start = 8.dp).size(18.dp),
        )
    }
}

/**
 * The open file's whole diff, laid out in line under its row: no cap and no
 * inner vertical scroll, so the screen's own scroll carries you through it.
 * Uses the diff the changes list already carried, else fetches it once.
 */
@Composable
private fun FileDiffBody(sessionId: String, row: ChangesRow.File, inlineDiff: String?, cache: MutableMap<String, String>) {
    var text by remember(row.path) { mutableStateOf(inlineDiff?.takeIf { it.isNotEmpty() } ?: cache[row.path]) }
    var loading by remember(row.path) { mutableStateOf(text == null) }
    var error by remember(row.path) { mutableStateOf<String?>(null) }

    LaunchedEffect(row.path, sessionId) {
        if (text != null) return@LaunchedEffect
        loading = true
        error = null
        try {
            val d: String? = withContext(Dispatchers.IO) { AppContainer.sessionRepository.loadChangeDiff(sessionId, row.path, row.turnIndex) }
            cache[row.path] = d.orEmpty()
            text = d.orEmpty()
        } catch (e: Exception) {
            error = e.message ?: "Failed to load diff."
        } finally {
            loading = false
        }
    }

    Column(modifier = Modifier.fillMaxWidth().background(NewTheme.Black)) {
        val body = text
        when {
            loading -> Text("Loading diff…", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(16.dp))
            error != null -> Banner(error ?: "Failed to load diff.")
            body.isNullOrEmpty() -> Note("No diff available for this file.")
            else -> {
                val diff = remember(body) { SharedNative.parseDiff(body) }
                DiffView(
                    diff = diff,
                    filePath = row.path,
                    maxCollapsedLines = Int.MAX_VALUE,
                    showFileHeader = false,
                )
            }
        }
    }
}
