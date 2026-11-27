package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import console.v1.QueuedPrompt
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowUp
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Trash

/**
 * The prompt staged for the next turn, shown above the composer: its text,
 * a "Queued" label, and send-now / edit / delete actions.
 */
@Composable
fun QueuedPromptCard(
    queued: QueuedPrompt,
    onSendNow: () -> Unit,
    onEdit: () -> Unit,
    onDelete: () -> Unit,
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 10.dp)
            .padding(bottom = 8.dp)
            .clip(RoundedCornerShape(NewTheme.CardRadius))
            .background(NewTheme.Card)
            .padding(start = 18.dp, end = 6.dp, top = 6.dp, bottom = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(
                text = queued.prompt.replace('\n', ' '),
                color = NewTheme.TextPrimary,
                fontSize = 15.sp,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            Text(
                text = "Queued",
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                modifier = Modifier.padding(top = 1.dp),
            )
        }
        Row(horizontalArrangement = Arrangement.spacedBy(2.dp), verticalAlignment = Alignment.CenterVertically) {
            QueueAction(TablerIcons.Outline.Edit, "Edit queued prompt", onEdit)
            QueueAction(TablerIcons.Outline.Trash, "Delete queued prompt", onDelete)
            QueueAction(TablerIcons.Outline.ArrowUp, "Send now", onSendNow)
        }
    }
}

@Composable
private fun QueueAction(icon: ImageVector, description: String, onClick: () -> Unit) {
    Box(
        modifier = Modifier.size(36.dp).clip(RoundedCornerShape(999.dp)).clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = description, tint = NewTheme.TextSecondary, modifier = Modifier.size(20.dp))
    }
}
