package com.console.mobile.feature.home

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.chat.isSessionTitlePending
import com.console.mobile.core.util.formatRelativeTime
import com.console.mobile.ui.components.common.new.StatusPill
import com.console.mobile.ui.components.common.new.TintPill
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionHeader
import com.console.mobile.data.model.SessionStatus
import kotlinx.coroutines.delay

/** Height of a row's first line: the title and the status pill are both centred in it. */
private val FirstLineHeight = 28.dp

@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun SessionRow(
    session: SessionHeader,
    projectName: String,
    branch: String?,
    status: SessionStatus?,
    isDraft: Boolean,
    draftPreview: String?,
    onClick: () -> Unit,
    onLongPress: () -> Unit,
) {
    // Top-aligned, with a fixed first line shared by the title and the status pill.
    // Centring the right column on the whole row put "Ready" lower than the "Draft"
    // pill beside the title; giving both the same line height puts their centres level.
    val titlePending = isSessionTitlePending(session)
    Row(
        modifier = Modifier.fillMaxWidth()
            .combinedClickable(onClick = onClick, onLongClick = onLongPress)
            .padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.Top,
    ) {
        Column(modifier = Modifier.weight(1f).padding(end = 12.dp)) {
            Row(modifier = Modifier.height(FirstLineHeight), verticalAlignment = Alignment.CenterVertically) {
                if (titlePending) {
                    // The server is generating the title (first prompt sent,
                    // placeholder still stored): pulse dots instead of the
                    // raw placeholder until sessionTitleUpdated renames it.
                    var dots by remember(session.id) { mutableStateOf(0) }
                    LaunchedEffect(session.id) {
                        while (true) {
                            delay(400)
                            dots = (dots + 1) % 4
                        }
                    }
                    Text(
                        "Generating title" + ".".repeat(dots),
                        color = NewTheme.TextMuted,
                        fontSize = 17.sp,
                        fontStyle = FontStyle.Italic,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                } else {
                    Text(
                        session.title,
                        color = NewTheme.TextPrimary,
                        fontSize = 17.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                }
                if (isDraft) TintPill("Draft", NewTheme.Warning, Modifier.padding(start = 8.dp))
            }
            if (isDraft && draftPreview != null) {
                Text(draftPreview, color = NewTheme.Warning, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
            }
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 1.dp)) {
                Text(projectName, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                if (branch != null) {
                    Text(" • ", color = NewTheme.TextMuted, fontSize = 13.sp)
                    Text(branch, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                }
            }
        }
        Column(horizontalAlignment = Alignment.End) {
            Box(modifier = Modifier.height(FirstLineHeight), contentAlignment = Alignment.CenterEnd) { StatusPill(status) }
            val ts = if (session.updated_at > 0) session.updated_at else session.created_at
            Text(shortRelative(ts), color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 1.dp))
        }
    }
}

private fun shortRelative(ts: Long): String {
    if (ts <= 0) return ""
    return formatRelativeTime(ts).replace(" ago", "").replace("just now", "now")
}
