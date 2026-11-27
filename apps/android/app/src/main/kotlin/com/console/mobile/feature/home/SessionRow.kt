package com.console.mobile.feature.home

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.ui.graphics.StrokeCap
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
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionHeader
import com.console.mobile.data.model.SessionStatus
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Bolt
import io.github.lyxnx.compose.ui.tablericons.outline.Calendar
import io.github.lyxnx.compose.ui.tablericons.outline.Pencil
import kotlinx.coroutines.delay

@OptIn(ExperimentalFoundationApi::class)
@Composable
internal fun SessionRow(
    session: SessionHeader,
    projectName: String,
    branch: String?,
    status: SessionStatus?,
    isDraft: Boolean,
    draftPreview: String?,
    hasAutomation: Boolean = false,
    hasWebhook: Boolean = false,
    indented: Boolean = true,
    onClick: () -> Unit,
    onLongPress: () -> Unit,
) {
    val titlePending = isSessionTitlePending(session)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(androidx.compose.foundation.shape.RoundedCornerShape(10.dp))
            .combinedClickable(onClick = onClick, onLongClick = onLongPress)
            .padding(horizontal = 12.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Leading status mark, matching desktop's sidebar row.
        Box(
            modifier = Modifier.padding(end = 10.dp).size(18.dp),
            contentAlignment = Alignment.Center,
        ) {
            when {
                status == SessionStatus.Working -> CircularProgressIndicator(
                    color = NewTheme.Accent,
                    strokeWidth = 2.dp,
                    strokeCap = StrokeCap.Round,
                    modifier = Modifier.size(14.dp),
                )
                status == SessionStatus.NeedsAttention -> Box(
                    modifier = Modifier.size(8.dp).clip(CircleShape).background(NewTheme.Danger),
                )
                showsUnreadDot(session, status) -> Box(
                    modifier = Modifier.size(8.dp).clip(CircleShape).background(NewTheme.Accent),
                )
                status == SessionStatus.Done -> Icon(
                    TablerIcons.Outline.Check,
                    contentDescription = "Ready",
                    tint = NewTheme.Success,
                    modifier = Modifier.size(14.dp),
                )
                else -> Box(
                    modifier = Modifier.size(6.dp).clip(CircleShape).background(NewTheme.TextGhost),
                )
            }
        }

        Column(modifier = Modifier.weight(1f).padding(end = 10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (titlePending) {
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
                        fontSize = 15.sp,
                        fontStyle = FontStyle.Italic,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                } else {
                    Text(
                        session.title,
                        color = NewTheme.TextPrimary,
                        fontSize = 15.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f, fill = false),
                    )
                }
                if (hasAutomation) {
                    Icon(
                        TablerIcons.Outline.Calendar,
                        contentDescription = "Scheduled automation",
                        tint = NewTheme.Accent,
                        modifier = Modifier.padding(start = 6.dp).size(13.dp),
                    )
                }
                if (hasWebhook) {
                    Icon(
                        TablerIcons.Outline.Bolt,
                        contentDescription = "Active webhook",
                        tint = NewTheme.Accent,
                        modifier = Modifier.padding(start = 5.dp).size(13.dp),
                    )
                }
                if (isDraft) {
                    Icon(
                        TablerIcons.Outline.Pencil,
                        contentDescription = "Unsent draft",
                        tint = NewTheme.TextMuted,
                        modifier = Modifier.padding(start = 6.dp).size(13.dp),
                    )
                }
            }
            when (session.launch_state) {
                "provisioning" -> Text(
                    "Creating worktree…",
                    color = NewTheme.Accent,
                    fontSize = 12.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 2.dp),
                )
                "failed" -> Text(
                    "Worktree failed",
                    color = NewTheme.Danger,
                    fontSize = 12.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 2.dp),
                )
                else -> if (isDraft && draftPreview != null) {
                    Text(
                        draftPreview,
                        color = NewTheme.Warning,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
        }

        val ts = if (session.updated_at > 0) session.updated_at else session.created_at
        Text(shortRelative(ts), color = NewTheme.TextMuted, fontSize = 12.sp)
    }
}

/**
 * Whether a row shows the unread dot: the server says the chat is unread, and
 * it is neither running nor waiting on the user, whose pills already say so.
 */
internal fun showsUnreadDot(session: SessionHeader, status: SessionStatus?): Boolean =
    session.unread && status != SessionStatus.Working && status != SessionStatus.NeedsAttention

private fun shortRelative(ts: Long): String {
    if (ts <= 0) return ""
    return formatRelativeTime(ts).replace(" ago", "").replace("just now", "now")
}
