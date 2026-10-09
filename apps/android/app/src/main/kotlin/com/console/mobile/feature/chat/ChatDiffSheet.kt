package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.AppContainer
import com.console.mobile.core.util.getFileName
import com.console.mobile.core.util.parseUnifiedDiff
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionFileChange

/**
 * In-chat modal bottom sheet for inspecting a turn's file change and unified diff.
 */
@Composable
fun ChatDiffSheet(
    change: SessionFileChange?,
    sessionId: String,
    cwd: String?,
    onDismiss: () -> Unit,
) {
    if (change == null) return

    var diffText by remember(change) { mutableStateOf<String?>(change.diff_text) }
    var loading by remember(change) { mutableStateOf(change.diff_text.isNullOrEmpty()) }
    var error by remember(change) { mutableStateOf<String?>(null) }

    LaunchedEffect(change, sessionId) {
        if (!change.diff_text.isNullOrEmpty()) {
            diffText = change.diff_text
            loading = false
            error = null
            return@LaunchedEffect
        }
        loading = true
        diffText = null
        error = null
        try {
            val d = AppContainer.sessionRepository.loadChangeDiff(sessionId, change.path, change.turn_index)
            diffText = d
        } catch (e: Exception) {
            error = e.message ?: "Failed to load diff."
        } finally {
            loading = false
        }
    }

    BaseSheet(
        onDismiss = onDismiss,
        title = getFileName(change.path),
    ) {
        val relPath = remember(change.path, cwd) {
            if (cwd != null && change.path.startsWith(cwd)) {
                change.path.removePrefix(cwd).trimStart('/')
            } else change.path
        }
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 4.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            FileIcon(filename = change.path, sizeDp = 18, modifier = Modifier.padding(end = 8.dp))
            Column(modifier = Modifier.weight(1f)) {
                Text(relPath, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            DiffSummaryBadge(addedCount = change.additions, removedCount = change.deletions)
        }
        SectionCard(modifier = Modifier.fillMaxWidth().padding(top = 10.dp)) {
            when {
                loading -> {
                    Box(modifier = Modifier.fillMaxWidth().padding(32.dp), contentAlignment = Alignment.Center) {
                        CircularProgressIndicator(color = NewTheme.TextPrimary, modifier = Modifier.size(24.dp), strokeWidth = 2.dp)
                    }
                }
                error != null -> {
                    Text(error ?: "Failed to load diff", color = NewTheme.Danger, fontSize = 13.sp, modifier = Modifier.padding(16.dp))
                }
                !diffText.isNullOrEmpty() -> {
                    DiffView(
                        diff = parseUnifiedDiff(diffText ?: ""),
                        filePath = change.path,
                        maxCollapsedLines = 80,
                    )
                }
                else -> {
                    Text("No diff available for this file.", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(16.dp))
                }
            }
        }
    }
}
