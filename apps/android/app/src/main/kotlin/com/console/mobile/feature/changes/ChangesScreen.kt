package com.console.mobile.feature.changes

import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.draw.clip
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
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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
        Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
            ScreenHeader(title = baseOf(sel), subtitle = stripRepoPrefix(sel, repoPath), onBack = { selectedPath = null })
            Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 12.dp).padding(bottom = 32.dp)) {
                if (change != null) {
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(bottom = 12.dp)) {
                        Text(statusLetter(change.status), color = parseChangeColor(statusColorHex(change.status)), fontSize = 12.sp, fontWeight = FontWeight.Bold)
                        Text("+${change.additions}", color = Color(0xFF34D399), fontSize = 12.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.padding(start = 8.dp))
                        Text("-${change.deletions}", color = Color(0xFFF87171), fontSize = 12.sp, fontFamily = ConsoleMonoFamily, modifier = Modifier.padding(start = 8.dp))
                        Box(modifier = Modifier.weight(1f))
                        Text("Reviewed", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(end = 4.dp))
                        Checkbox(
                            checked = change.reviewed,
                            onCheckedChange = {
                                sessionId?.let { sid ->
                                    AppContainer.sessionRepository.toggleChangeReviewed(sid, change.path, change.turn_index, !change.reviewed)
                                }
                            },
                        )
                    }
                }
                when {
                    diffLoading -> Box(modifier = Modifier.fillMaxWidth().padding(vertical = 40.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(color = ConsoleColors.TextMuted)
                    }
                    diffError != null -> Box(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Color.Black.copy(alpha = 0.4f)).padding(12.dp)) {
                        Text(diffError ?: "Failed to load diff.", color = ConsoleColors.Destructive, fontSize = 12.sp)
                    }
                    diffText != null -> DiffView(diff = parseUnifiedDiff(diffText ?: ""), filePath = sel)
                    else -> Box(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(12.dp)).background(Color.Black.copy(alpha = 0.4f)).padding(12.dp)) {
                        Text("No diff available for this file.", color = ConsoleColors.TextSecondary, fontSize = 12.sp)
                    }
                }
            }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Changes",
            subtitle = "${totals.files} files  +${totals.additions} -${totals.deletions}",
            onBack = onBack,
            actions = {
                IconButton(onClick = ::refresh, modifier = Modifier.size(40.dp)) {
                    Icon(TablerIcons.Outline.Refresh, contentDescription = "Refresh", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
                }
            },
        )
        when {
            sessionId == null -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("No active session", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                    Text("Open a chat to see its file changes.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                }
            }
            loading -> Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    CircularProgressIndicator(color = ConsoleColors.TextMuted)
                    Text("Loading changes…", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 8.dp))
                }
            }
            error != null -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("Couldn't load changes", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                    Text(error ?: "Failed to load session changes.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                }
            }
            rows.isEmpty() -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Text("No file changes yet", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                    Text("Files this session touches will show up here.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                }
            }
            else -> LazyColumn(modifier = Modifier.fillMaxSize().padding(bottom = 24.dp)) {
                items(rows, key = { it.keyOf() }) { row ->
                    when (row) {
                        is ChangesRow.Folder -> Row(
                            modifier = Modifier.fillMaxWidth().clickable {
                                collapsed = if (collapsed.contains(row.name)) collapsed - row.name else collapsed + row.name
                            }.padding(horizontal = 16.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Icon(if (collapsed.contains(row.name)) TablerIcons.Outline.ChevronRight else TablerIcons.Outline.ChevronUp, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
                            Text("${row.name} · ${row.count}", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 6.dp))
                            DiffSummaryBadge(addedCount = row.additions, removedCount = row.deletions)
                        }
                        is ChangesRow.File -> Row(
                            modifier = Modifier.fillMaxWidth().clickable { selectedPath = row.path }.padding(horizontal = 16.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(statusLetter(row.status), color = parseChangeColor(statusColorHex(row.status)), fontSize = 11.sp, fontWeight = FontWeight.Bold, modifier = Modifier.size(12.dp))
                            FileIcon(filename = row.name, sizeDp = 16)
                            Column(modifier = Modifier.weight(1f).padding(start = 8.dp)) {
                                Text(row.name, color = ConsoleColors.TextPrimary, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                Text(row.rel, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                            DiffSummaryBadge(addedCount = row.additions, removedCount = row.deletions)
                            Checkbox(
                                checked = row.reviewed,
                                onCheckedChange = {
                                    sessionId?.let { sid ->
                                        AppContainer.sessionRepository.toggleChangeReviewed(sid, row.path, row.turnIndex, !row.reviewed)
                                    }
                                },
                            )
                            Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
                        }
                    }
                }
            }
        }
    }
}

private fun ChangesRow.keyOf(): String = when (this) {
    is ChangesRow.Folder -> key
    is ChangesRow.File -> key
}

private fun parseChangeColor(hex: String): Color {
    return try {
        Color(android.graphics.Color.parseColor(hex))
    } catch (_: Exception) {
        ConsoleColors.TextMuted
    }
}
