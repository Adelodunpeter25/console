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
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
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
import console.v1.FileSearchResult
import console.v1.SlashCommandInfo
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
    onAddProject: () -> Unit,
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
    if (fieldValue.text != value) {
        fieldValue = fieldValue.copy(text = value, selection = TextRange(minOf(fieldValue.selection.start, value.length)))
    }
    // Paths inserted from the autocomplete popup. Only these count as
    // confirmed chips for atomic backspace — freshly typed `@query` text
    // (even when it parses as a mention) keeps char-by-char deletion.
    var confirmedPaths by remember(sessionId) { mutableStateOf(setOf<String>()) }
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

    fun applyFileSuggestion(file: console.v1.FileSearchResult, replaceFrom: Int) {
        confirmedPaths = confirmedPaths + file.relative_path
        applySuggestion("@${file.relative_path} ", replaceFrom)
    }

    val pickImages = rememberAttachmentPicker(sessionId)

    // ime minus nav bars: Scaffold already pads the nav bar, so only lift
    // by the keyboard itself — otherwise the gap doubles when typing.
    Column(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Background).windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)).padding(horizontal = 10.dp).padding(top = 8.dp, bottom = 8.dp)) {
        if (topBanner != null) topBanner()
        ComposerTopStrip(sessionId = sessionId, running = running, projectLocked = projectLocked, onAddProject = onAddProject)
        if (attachments.isNotEmpty()) {
            AttachmentStrip(sessionId = sessionId, attachments = attachments)
        }
        ComposerInput(
            value = value,
            fieldValue = fieldValue,
            mentionVisual = mentionVisual,
            onFieldValueChange = { new ->
                // Chip-atomic backspace: a single delete ending inside a
                // confirmed mention removes the whole `@path` instead of
                // one character (which would drop back to plain text).
                // Freshly typed queries are never confirmed, so typing
                // keeps char-by-char deletion.
                val old = fieldValue
                var nextText = new.text
                var nextSelection: TextRange? = null
                if (old.selection.collapsed && new.selection.collapsed &&
                    new.text.length == old.text.length - 1 &&
                    new.selection.start + 1 == old.selection.start
                ) {
                    val deletedAt = new.selection.start
                    if (deletedAt >= 0 && deletedAt < old.text.length &&
                        old.text.removeRange(deletedAt, deletedAt + 1) == new.text
                    ) {
                        val mentions = parseFileMentions(old.text)
                        // Direct hit: deletion ended inside a confirmed chip.
                        var target = mentions
                            .firstOrNull { deletedAt in it.range && it.path in confirmedPaths }
                        var removeEnd = target?.let { it.range.last + 1 }
                        // Separator hit: backspace took the whitespace right
                        // after a confirmed chip (usually the space the
                        // popup inserted) — take the chip with it so one
                        // press removes the whole pill, not just the space.
                        if (target == null && old.text[deletedAt].isWhitespace()) {
                            target = mentions.firstOrNull {
                                it.range.last + 1 == deletedAt && it.path in confirmedPaths
                            }
                            removeEnd = target?.let { deletedAt + 1 }
                        }
                        if (target != null && removeEnd != null) {
                            nextText = old.text.removeRange(target.range.first, removeEnd)
                            nextSelection = TextRange(target.range.first)
                        }
                    }
                }
                // Prune confirmations whose text is gone.
                confirmedPaths = confirmedPaths.filter { "@$it" in nextText }.toSet()
                fieldValue = if (nextSelection != null) {
                    TextFieldValue(text = nextText, selection = nextSelection)
                } else {
                    new
                }
                onChange(nextText)
            },
            onCoordinatesChange = { fieldCoordinates = it },
            attach = { pickImages() },
            sessionId = sessionId,
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
                                    applyFileSuggestion(file, t.start)
                                }
                            }
                        }
                    }
                }
                null -> {}
            }
        }
        ComposerBottomStrip(sessionId = sessionId)
    }
}

private val INPUT_LINE_HEIGHT = 19.sp

/** The input bubble: attach button, text field, and send/stop. */
@Composable
private fun ComposerInput(
    value: String,
    fieldValue: TextFieldValue,
    mentionVisual: MentionVisual,
    onFieldValueChange: (TextFieldValue) -> Unit,
    onCoordinatesChange: (LayoutCoordinates) -> Unit,
    attach: () -> Unit,
    sessionId: String,
    running: Boolean,
    canSend: Boolean,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    // A constant rounded rect: the old pill-to-rect morph keyed off the visual
    // line count and made the bubble jump shape while typing.
    val bubbleShape = RoundedCornerShape(20.dp)
    // Two lines of text from the start (three lines with the button row), so the
    // bubble doesn't grow as you type.
    // Derived from the text's own line height so it tracks the system font size.
    val inputMinHeight = with(androidx.compose.ui.platform.LocalDensity.current) { (INPUT_LINE_HEIGHT * 2).toDp() }
    Column(
        modifier = Modifier.fillMaxWidth().clip(bubbleShape)
            .background(ConsoleColors.Card)
            .border(1.dp, ConsoleColors.Border, bubbleShape)
            .onGloballyPositioned(onCoordinatesChange)
            .padding(horizontal = 6.dp, vertical = 6.dp),
    ) {
        var mentionLayout by remember { mutableStateOf<androidx.compose.ui.text.TextLayoutResult?>(null) }
        var fieldHeightPx by remember { mutableStateOf(0) }
        // The editable field cannot host icon glyphs, so mention icons ride
        // as an overlay on each collapsed mention's reserved slot. Hidden
        // while the field scrolls internally (layout taller than the box),
        // where slot coordinates would no longer line up.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 8.dp, vertical = 6.dp)
                .heightIn(min = inputMinHeight, max = maxOf(80.dp, inputMinHeight))
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
                    lineHeight = INPUT_LINE_HEIGHT,
                ),
                cursorBrush = SolidColor(ConsoleColors.TextPrimary),
                visualTransformation = mentionVisual.asTransformation(),
                maxLines = 6,
                onTextLayout = { mentionLayout = it },
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        val layout = mentionLayout
                        val slots = mentionVisual.segments
                        if (layout != null && slots.isNotEmpty() && layout.size.height <= fieldHeightPx) {
                            val density = androidx.compose.ui.platform.LocalDensity.current
                            slots.forEach { seg ->
                                if (seg.tEnd <= layout.layoutInput.text.length && seg.tSlot < seg.tEnd) {
                                    val startBox = layout.getBoundingBox(seg.tSlot)
                                    val endBox = layout.getBoundingBox((seg.tEnd - 1).coerceAtLeast(seg.tSlot))
                                    val pillHeight = with(density) { 18.dp.toPx() }
                                    Box(
                                        modifier = Modifier
                                            .offset {
                                                androidx.compose.ui.unit.IntOffset(
                                                    (startBox.left - with(density) { 2.dp.toPx() }).roundToInt(),
                                                    (startBox.top + (startBox.height - pillHeight) / 2).roundToInt(),
                                                )
                                            }
                                            .width(with(density) { (endBox.right - startBox.left + 5.dp.toPx()).toDp() })
                                            .height(18.dp)
                                            .clip(RoundedCornerShape(4.dp))
                                            .background(MentionAccent.copy(alpha = 0.12f))
                                            .border(1.dp, MentionAccent.copy(alpha = 0.28f), RoundedCornerShape(4.dp)),
                                    )
                                }
                            }
                        }
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
                    if (seg.tEnd <= layout.layoutInput.text.length && seg.tSlot < seg.tEnd) {
                        val box = layout.getBoundingBox(seg.tSlot)
                        val iconPx = with(density) { 12.dp.roundToPx() }
                        Box(
                            modifier = Modifier
                                .offset {
                                    androidx.compose.ui.unit.IntOffset(
                                        (box.left + with(density) { 1.dp.toPx() }).roundToInt(),
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
        }
        Row(
            modifier = Modifier.fillMaxWidth().padding(top = 2.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier.size(35.dp).clip(CircleShape).clickable(onClickLabel = "Attach image", onClick = attach),
                contentAlignment = Alignment.Center,
            ) {
                androidx.compose.material3.Icon(TablerIcons.Outline.Plus, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(20.dp))
            }
            // Fills the gap so send/stop stays pinned right; the chip truncates
            // rather than pushing the button off a narrow screen.
            Row(
                modifier = Modifier.weight(1f).padding(horizontal = 4.dp),
                horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(6.dp, Alignment.End),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                // Model shrinks first (its label truncates); the thinking chip is short and stays whole.
                ModelChip(sessionId = sessionId, modifier = Modifier.weight(1f, fill = false))
                ThinkingChip(sessionId = sessionId, running = running)
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
}
