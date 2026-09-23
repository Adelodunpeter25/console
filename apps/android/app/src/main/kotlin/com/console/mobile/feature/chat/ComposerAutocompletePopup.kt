package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.boundsInWindow
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.IntRect
import androidx.compose.ui.unit.IntSize
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupPositionProvider
import com.console.mobile.data.model.FileSearchResult
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily

private val MentionAccent = Color(0xFF60A5FA)

/** Floating suggestion list anchored above [anchor] (the composer input), with a real dp gap. */
@Composable
fun ComposerAutocompletePopup(anchor: LayoutCoordinates, gap: Dp = 10.dp, content: @Composable () -> Unit) {
    val density = LocalDensity.current
    val gapPx = with(density) { gap.roundToPx() }
    Popup(
        popupPositionProvider = remember(anchor, gapPx) {
            object : PopupPositionProvider {
                override fun calculatePosition(anchorBounds: IntRect, windowSize: IntSize, layoutDirection: LayoutDirection, popupContentSize: IntSize): IntOffset {
                    val bounds = anchor.boundsInWindow()
                    val x = bounds.left.toInt().coerceIn(0, maxOf(0, windowSize.width - popupContentSize.width))
                    val y = (bounds.top.toInt() - popupContentSize.height - gapPx).coerceAtLeast(0)
                    return IntOffset(x, y)
                }
            }
        },
        onDismissRequest = {},
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 10.dp)
                .clip(RoundedCornerShape(14.dp))
                .background(ConsoleColors.Card)
                .border(1.dp, ConsoleColors.Border, RoundedCornerShape(14.dp))
                .heightIn(max = 260.dp)
                .verticalScroll(rememberScrollState())
                .padding(vertical = 6.dp),
        ) { content() }
    }
}

@Composable
fun SlashCommandSuggestionRow(command: SlashCommandInfo, onClick: () -> Unit) {
    Column(modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 14.dp, vertical = 8.dp)) {
        Text("/${command.name}", color = ConsoleColors.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.SemiBold, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis)
        if (command.description.isNotBlank()) {
            Text(command.description, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
        }
    }
}

/** File-mention suggestion row — icon + filename chip (mirrors desktop's file_mention_chip), full path muted alongside. */
@Composable
fun FileMentionSuggestionRow(file: FileSearchResult, onClick: () -> Unit) {
    val filename = file.relativePath.substringAfterLast('/')
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 10.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            modifier = Modifier
                .clip(RoundedCornerShape(4.dp))
                .background(MentionAccent.copy(alpha = 0.10f))
                .border(1.dp, MentionAccent.copy(alpha = 0.28f), RoundedCornerShape(4.dp))
                .padding(horizontal = 6.dp, vertical = 3.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            FileIcon(filename = filename, sizeDp = 12)
            Text(filename, color = MentionAccent, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 4.dp))
        }
        if (file.relativePath != filename) {
            Text(file.relativePath, color = ConsoleColors.TextMuted, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 8.dp))
        }
    }
}
