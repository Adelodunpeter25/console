package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.onGloballyPositioned
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.util.ComposerTrigger
import com.console.mobile.core.util.detectComposerTrigger
import com.console.mobile.core.util.parseFileMentions
import com.console.mobile.data.model.FileSearchResult
import com.console.mobile.data.model.SlashCommandInfo
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleColors
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerStop
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Send
import kotlinx.coroutines.delay
import kotlin.math.roundToInt

/**
 * Port of components/chat/composer (composer + composer-input).
 * Multiline input with a send/stop button, image attach, and the
 * slash-command / @file autocomplete. The chips below live in
 * [ComposerBottomStrip], attachments in [ComposerAttachments].
 */
@Composable
fun Composer(
    sessionId: String,
    value: String,
    onChange: (String) -> Unit,
    running: Boolean,
    projectLocked: Boolean,
    topBanner: (@Composable () -> Unit)? = null,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val attachments = chatSessions[sessionId]?.attachments ?: emptyList()
    val canSend = value.trim().isNotEmpty() || attachments.isNotEmpty()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionCwd = sessionViews[sessionId]?.sessionCwd
    val projectRoot = projectState.projects.firstOrNull { p -> sessionCwd != null && (p.path == sessionCwd || sessionCwd.startsWith(p.path + "/")) }?.path ?: sessionCwd

    var fieldValue by remember(sessionId) { mutableStateOf(TextFieldValue(text = value, selection = TextRange(value.length))) }
    var visualLines by remember(sessionId) { mutableStateOf(1) }
    if (fieldValue.text != value) {
        fieldValue = fieldValue.copy(text = value, selection = TextRange(minOf(fieldValue.selection.start, value.length)))
    }
    var fieldCoordinates by remember { mutableStateOf<LayoutCoordinates?>(null) }
    val trigger = remember(fieldValue) { detectComposerTrigger(fieldValue.text, fieldValue.selection.start) }
    // Accent-wash styling for @-mention ranges (desktop file_mention_chip parity).
    // The in-progress autocomplete query keeps its raw `@query` text — only
    // confirmed mentions (picked from the popup) collapse to filename pills.
    val confirmedMentions = remember(fieldValue.text, trigger) {
        val all = parseFileMentions(fieldValue.text)
        val t = trigger
        if (t is ComposerTrigger.Mention) {
            val cursor = fieldValue.selection.start
            all.filterNot { it.range.first < cursor && it.range.last + 1 > t.start }
        } else {
            all
        }
    }
    val mentionVisual = remember(fieldValue.text, confirmedMentions) {
        buildMentionVisual(fieldValue.text, confirmedMentions)
    }

    var slashCommands by remember(sessionId) { mutableStateOf<List<SlashCommandInfo>>(emptyList()) }
    LaunchedEffect(sessionId, trigger is ComposerTrigger.Slash) {
        if (trigger is ComposerTrigger.Slash && slashCommands.isEmpty()) {
            slashCommands = AppContainer.assistRepository.listSlashCommands(sessionId)
        }
    }
    var mentionResults by remember { mutableStateOf<List<FileSearchResult>>(emptyList()) }
    LaunchedEffect(trigger) {
        val t = trigger
        if (t is ComposerTrigger.Mention) {
            delay(200)
            mentionResults = AppContainer.assistRepository.searchMentionFiles(sessionId, t.query, projectRoot)
        } else {
            mentionResults = emptyList()
        }
    }

    fun applySuggestion(insert: String, replaceFrom: Int) {
        val current = fieldValue.text
        val cursor = fieldValue.selection.start.coerceIn(0, current.length)
        val newText = current.substring(0, replaceFrom) + insert + current.substring(cursor)
        fieldValue = TextFieldValue(text = newText, selection = TextRange(replaceFrom + insert.length))
        onChange(newText)
    }

    val pickImages = rememberAttachmentPicker(sessionId)

    // ime minus nav bars: Scaffold already pads the nav bar, so only lift
    // by the keyboard itself — otherwise the gap doubles when typing.
    Column(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Background).windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)).padding(horizontal = 10.dp).padding(top = 8.dp, bottom = 8.dp)) {
        if (topBanner != null) topBanner()
        if (attachments.isNotEmpty()) {
            AttachmentStrip(sessionId = sessionId, attachments = attachments)
        }
        ComposerInput(
            value = value,
            fieldValue = fieldValue,
            visualLines = visualLines,
            mentionVisual = mentionVisual,
            onFieldValueChange = { new ->
                fieldValue = new
                onChange(new.text)
            },
            onVisualLinesChange = { visualLines = it },
            onCoordinatesChange = { fieldCoordinates = it },
            attach = { pickImages() },
            running = running,
            canSend = canSend,
            onSend = onSend,
            onStop = onStop,
        )
        val anchor = fieldCoordinates
        if (anchor != null) {
            when (val t = trigger) {
                is ComposerTrigger.Slash -> {
                    val items = slashCommands.filter { it.name.contains(t.query, ignoreCase = true) }
                    if (items.isNotEmpty()) {
                        ComposerAutocompletePopup(anchor = anchor) {
                            items.take(20).forEach { cmd ->
                                SlashCommandSuggestionRow(command = cmd) {
                                    applySuggestion("/${cmd.name} ", t.start)
                                }
                            }
                        }
                    }
                }
                is ComposerTrigger.Mention -> {
                    if (mentionResults.isNotEmpty()) {
                        ComposerAutocompletePopup(anchor = anchor) {
                            mentionResults.take(20).forEach { file ->
                                FileMentionSuggestionRow(file = file) {
                                    applySuggestion("@${file.relativePath} ", t.start)
                                }
                            }
                        }
                    }
                }
                null -> {}
            }
        }
        ComposerBottomStrip(sessionId = sessionId, projectLocked = projectLocked)
    }
}

/** The input bubble: attach button, text field, and send/stop. */
@Composable
private fun ComposerInput(
    value: String,
    fieldValue: TextFieldValue,
    visualLines: Int,
    mentionVisual: MentionVisual,
    onFieldValueChange: (TextFieldValue) -> Unit,
    onVisualLinesChange: (Int) -> Unit,
    onCoordinatesChange: (LayoutCoordinates) -> Unit,
    attach: () -> Unit,
    running: Boolean,
    canSend: Boolean,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    // Rounded rect as soon as the bubble grows past one *visual* line.
    // Keying off "\n" alone missed word-wrap, which adds no newline char,
    // so wrapped text kept the pill while shift+enter flipped to the rect.
    val bubbleShape = if (visualLines > 1) RoundedCornerShape(20.dp) else CircleShape
    Row(
        modifier = Modifier.fillMaxWidth().clip(bubbleShape)
            .background(ConsoleColors.Card)
            .border(1.dp, ConsoleColors.Border, bubbleShape)
            .onGloballyPositioned(onCoordinatesChange)
            .padding(horizontal = 6.dp, vertical = 6.dp),
        verticalAlignment = Alignment.Bottom,
    ) {
        Box(
            modifier = Modifier.size(37.dp).clip(CircleShape).clickable(onClickLabel = "Attach image", onClick = attach),
            contentAlignment = Alignment.Center,
        ) {
            androidx.compose.material3.Icon(TablerIcons.Outline.Plus, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(20.dp))
        }
        var mentionLayout by remember { mutableStateOf<androidx.compose.ui.text.TextLayoutResult?>(null) }
        var fieldHeightPx by remember { mutableStateOf(0) }
        // The editable field cannot host icon glyphs, so mention icons ride
        // as an overlay on each collapsed mention's reserved slot. Hidden
        // while the field scrolls internally (layout taller than the box),
        // where slot coordinates would no longer line up.
        Box(
            modifier = Modifier
                .align(Alignment.CenterVertically)
                .weight(1f)
                .padding(horizontal = 4.dp)
                .heightIn(max = 120.dp)
                .onGloballyPositioned { fieldHeightPx = it.size.height }
                .clipToBounds(),
        ) {
            BasicTextField(
                value = fieldValue,
                onValueChange = onFieldValueChange,
                modifier = Modifier.fillMaxWidth(),
                textStyle = androidx.compose.ui.text.TextStyle(
                    color = ConsoleColors.TextPrimary,
                    fontSize = 14.sp,
                    lineHeight = 19.sp,
                ),
                cursorBrush = SolidColor(ConsoleColors.TextPrimary),
                visualTransformation = mentionVisual.asTransformation(),
                maxLines = 6,
                onTextLayout = {
                    mentionLayout = it
                    onVisualLinesChange(it.lineCount)
                },
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty()) {
                            Text("Ask anything…", color = ConsoleColors.TextMuted, fontSize = 14.sp)
                        }
                        innerTextField()
                    }
                },
            )
            val layout = mentionLayout
            val slots = mentionVisual.segments
            if (layout != null && slots.isNotEmpty() && layout.size.height <= fieldHeightPx) {
                val density = androidx.compose.ui.platform.LocalDensity.current
                slots.forEach { seg ->
                    val box = layout.getBoundingBox(seg.tSlot)
                    val iconPx = with(density) { 12.dp.roundToPx() }
                    Box(
                        modifier = Modifier
                            .offset {
                                androidx.compose.ui.unit.IntOffset(
                                    box.left.roundToInt(),
                                    (box.top + (box.height - iconPx) / 2).roundToInt(),
                                )
                            }
                            .size(12.dp),
                    ) {
                        FileIcon(filename = seg.path.substringAfterLast('/'), sizeDp = 12)
                    }
                }
            }
        }
        if (running) {
            // Same 35dp footprint as the send button so the composer doesn't
            // resize mid-send, and destructive red so the control's meaning
            // is readable at a glance rather than only from a tiny glyph.
            Box(
                modifier = Modifier.size(35.dp).clip(CircleShape).background(ConsoleColors.Destructive)
                    .clickable(onClickLabel = "Stop", onClick = onStop),
                contentAlignment = Alignment.Center,
            ) {
                androidx.compose.material3.Icon(TablerIcons.Outline.PlayerStop, contentDescription = "Stop generating", tint = Color.Black, modifier = Modifier.size(14.dp))
            }
        } else {
            Box(
                modifier = Modifier.size(35.dp).clip(CircleShape)
                    .background(if (canSend) Color.White else Color.White.copy(alpha = 0.08f))
                    .clickable(enabled = canSend, onClickLabel = "Send", onClick = onSend),
                contentAlignment = Alignment.Center,
            ) {
                androidx.compose.material3.Icon(TablerIcons.Outline.Send, contentDescription = null, tint = if (canSend) Color.Black else ConsoleColors.TextMuted, modifier = Modifier.size(15.dp))
            }
        }
    }
}
