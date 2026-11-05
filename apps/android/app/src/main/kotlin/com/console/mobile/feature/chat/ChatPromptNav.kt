package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.util.formatTimeAgo
import com.console.mobile.data.model.SessionPrompt
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowNarrowDown
import io.github.lyxnx.compose.ui.tablericons.outline.List as ListIcon

/**
 * Desktop's floating pill: a prompt-list button, plus a scroll-to-bottom arrow
 * while the reader is away from the latest message. Each half is its own tap
 * target and highlights as a whole section.
 */
@Composable
internal fun ChatScrollPill(
    showScrollToBottom: Boolean,
    onOpenPrompts: () -> Unit,
    onScrollToBottom: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(17.dp)
    Row(
        modifier = modifier.height(34.dp).clip(shape).background(NewTheme.Raised)
            .border(1.dp, Color.White.copy(alpha = 0.1f), shape),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        PillSection(TablerIcons.Outline.ListIcon, "Jump to prompt", onOpenPrompts)
        if (showScrollToBottom) {
            Box(modifier = Modifier.width(1.dp).height(15.dp).background(Color.White.copy(alpha = 0.1f)))
            PillSection(TablerIcons.Outline.ArrowNarrowDown, "Scroll to bottom", onScrollToBottom)
        }
    }
}

@Composable
private fun PillSection(icon: ImageVector, description: String, onClick: () -> Unit) {
    Box(
        modifier = Modifier.width(38.dp).height(34.dp).clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = description, tint = NewTheme.TextSecondary, modifier = Modifier.size(15.dp))
    }
}

/**
 * Every prompt of the chat, oldest first, opening on the newest. The list
 * comes from the server (so it covers history not paged in yet) and is cached
 * by the caller; [prompts] is null until the first fetch lands. A tap reports
 * the prompt's position counted from the newest (0 = newest) rather than an id,
 * because prompts sent this session have no server id on the client yet.
 */
@Composable
internal fun PromptsSheet(
    prompts: List<SessionPrompt>?,
    onSelect: (fromEnd: Int) -> Unit,
    onDismiss: () -> Unit,
) {
    BaseSheet(onDismiss = onDismiss, scrollable = false) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(start = 4.dp, end = 4.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text("Prompts", color = NewTheme.TextPrimary, fontSize = 20.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
            if (!prompts.isNullOrEmpty()) {
                Text("${prompts.size} ${if (prompts.size == 1) "prompt" else "prompts"}", color = NewTheme.TextMuted, fontSize = 14.sp)
            }
        }
        if (prompts == null) {
            Text("Loading…", color = NewTheme.TextMuted, fontSize = 15.sp, modifier = Modifier.padding(4.dp))
            return@BaseSheet
        }
        val listState = rememberLazyListState(initialFirstVisibleItemIndex = (prompts.size - 1).coerceAtLeast(0))
        // Newest at the bottom, in view; the oversized offset clamps at the end.
        LaunchedEffect(prompts.size) {
            if (prompts.isNotEmpty()) listState.scrollToItem(prompts.size - 1, Int.MAX_VALUE)
        }
        LazyColumn(state = listState, modifier = Modifier.weight(1f, fill = false)) {
            itemsIndexed(prompts, key = { _, prompt -> prompt.id }) { index, prompt ->
                Row(
                    modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(10.dp))
                        .clickable { onSelect(prompts.size - 1 - index) }.padding(horizontal = 4.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.Top,
                ) {
                    Text(
                        prompt.preview,
                        color = NewTheme.TextPrimary,
                        fontSize = 15.sp,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f),
                    )
                    if (prompt.createdAt > 0) {
                        Spacer(modifier = Modifier.width(12.dp))
                        Text(formatTimeAgo(prompt.createdAt), color = NewTheme.TextMuted, fontSize = 13.sp)
                    }
                }
            }
        }
    }
}
