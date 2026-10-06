package com.console.mobile.feature.chat

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
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.util.parseUnifiedDiff
import com.console.mobile.core.util.statusColorHex
import com.console.mobile.core.util.statusLetter
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Files
import kotlinx.coroutines.launch

/**
 * Per-session file changes for the active chat: rows with status,
 * add/remove counts, a reviewed toggle, and an expandable unified diff.
 * Backed by GET /api/sessions/:id/changes (+ diff + reviewed toggle).
 */
private fun changeColor(status: String): Color = try {
    Color(android.graphics.Color.parseColor(statusColorHex(status)))
} catch (_: Exception) {
    ConsoleColors.TextMuted
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SessionChangesSheet(sessionId: String, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    val changesBySession by AppContainer.sessionStateHolder.sessionChanges.collectAsStateWithLifecycle()
    val changes = changesBySession[sessionId].orEmpty()
    var loading by remember(sessionId) { mutableStateOf(true) }
    var expandedPath by remember(sessionId) { mutableStateOf<String?>(null) }
    val diffCache = remember(sessionId) { mutableStateMapOf<String, String?>() }
    var diffLoadingPath by remember(sessionId) { mutableStateOf<String?>(null) }

    LaunchedEffect(sessionId) {
        loading = true
        AppContainer.sessionRepository.loadSessionChanges(sessionId)
        loading = false
    }

    fun loadDiff(path: String, turnIndex: Int) {
        if (diffCache.containsKey(path) || diffLoadingPath != null) return
        diffLoadingPath = path
        scope.launch {
            try {
                diffCache[path] = AppContainer.sessionRepository.loadChangeDiff(sessionId, path, turnIndex)
            } catch (_: Exception) {
                diffCache[path] = null
            } finally {
                diffLoadingPath = null
            }
        }
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = ConsoleColors.Background,
    ) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(TablerIcons.Outline.Files, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
                Text(
                    "Session changes (${changes.size})",
                    color = ConsoleColors.TextPrimary,
                    fontSize = 16.sp,
                    fontWeight = FontWeight.SemiBold,
                    modifier = Modifier.padding(start = 8.dp),
                )
            }
            when {
                loading && changes.isEmpty() -> Box(
                    modifier = Modifier.fillMaxWidth().padding(vertical = 24.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator(color = ConsoleColors.TextMuted)
                }
                changes.isEmpty() -> Text(
                    "No file changes in this session yet.",
                    color = ConsoleColors.TextSecondary,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(vertical = 16.dp),
                )
                else -> LazyColumn(
                    state = rememberLazyListState(),
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                ) {
                    items(changes, key = { "${it.path}:${it.turn_index}" }) { change ->
                        val expanded = expandedPath == change.path
                        Column(
                            modifier = Modifier.fillMaxWidth().clickable {
                                if (expanded) {
                                    expandedPath = null
                                } else {
                                    expandedPath = change.path
                                    loadDiff(change.path, change.turn_index)
                                }
                            }.padding(vertical = 10.dp),
                        ) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    statusLetter(change.status),
                                    color = changeColor(change.status),
                                    fontSize = 11.sp,
                                    fontWeight = FontWeight.Bold,
                                    modifier = Modifier.size(14.dp),
                                )
                                Column(modifier = Modifier.weight(1f).padding(start = 8.dp)) {
                                    Text(change.path, color = ConsoleColors.TextPrimary, fontSize = 13.sp, maxLines = 1, overflow = androidx.compose.ui.text.style.TextOverflow.Ellipsis)
                                    Text(
                                        "turn ${change.turn_index}",
                                        color = ConsoleColors.TextSecondary,
                                        fontSize = 11.sp,
                                    )
                                }
                                DiffSummaryBadge(addedCount = change.additions, removedCount = change.deletions)
                                Checkbox(
                                    checked = change.reviewed,
                                    onCheckedChange = {
                                        AppContainer.sessionRepository.toggleChangeReviewed(
                                            sessionId,
                                            change.path,
                                            change.turn_index,
                                            !change.reviewed,
                                        )
                                    },
                                )
                            }
                            if (expanded) {
                                val cached = diffCache[change.path]
                                when {
                                    diffLoadingPath == change.path -> Box(
                                        modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
                                        contentAlignment = Alignment.Center,
                                    ) {
                                        CircularProgressIndicator(color = ConsoleColors.TextMuted, modifier = Modifier.size(18.dp))
                                    }
                                    cached != null -> DiffView(diff = parseUnifiedDiff(cached), filePath = change.path)
                                    else -> Text(
                                        "Diff unavailable.",
                                        color = ConsoleColors.TextSecondary,
                                        fontSize = 12.sp,
                                        modifier = Modifier.padding(vertical = 8.dp),
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
