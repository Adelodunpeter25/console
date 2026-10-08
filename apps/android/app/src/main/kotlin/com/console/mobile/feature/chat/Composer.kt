package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.layout.Spacer
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
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
import com.console.mobile.ui.theme.NewTheme
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

    // Collapsed to a one-line pill until the field is focused. Things the composer
    // launches (sheets, the image picker) steal window focus; [hold] keeps it open
    // for as long as they are showing so it doesn't fold up underneath them.
    val hold = remember { ComposerHold() }
    val pickImages = rememberAttachmentPicker(sessionId, hold)
    val focusRequester = remember { FocusRequester() }
    val focusManager = LocalFocusManager.current
    val keyboard = LocalSoftwareKeyboardController.current
    // The user's intent, not raw focus. A sheet or the picker steals window focus,
    // which would otherwise look like "the user left"; focus changes seen while
    // something is held are ignored, so intent survives and is restored after.
    var wantsOpen by remember(sessionId) { mutableStateOf(false) }
    // The composer's openness follows the keyboard's own animated height, so the
    // two move together instead of one after the other. The reference is the
    // tallest keyboard seen (a plain holder, not state: it is only read here).
    val density = androidx.compose.ui.platform.LocalDensity.current
    val imeBottomPx = androidx.compose.foundation.layout.WindowInsets.ime.getBottom(density)
    val imeRef = remember { intArrayOf(with(density) { DEFAULT_IME_HEIGHT_DP.dp.roundToPx() }) }
    if (imeBottomPx > imeRef[0]) imeRef[0] = imeBottomPx
    val imeOpen = imeBottomPx > 0
    val imeOpenNow = androidx.compose.runtime.rememberUpdatedState(imeOpen)

    fun focusField() {
        try {
            focusRequester.requestFocus()
            keyboard?.show()
        } catch (_: Exception) {
            // Not attached yet; the next tap will do it.
        }
    }
    // Open for reasons other than the keyboard: a sheet/picker is showing, focus
    // is coming back to the field after one closed (the keyboard has not started
    // yet), or the field is focused with no on-screen keyboard (hardware keyboard).
    var restoring by remember { mutableStateOf(false) }
    var focusedWithoutIme by remember(sessionId) { mutableStateOf(false) }
    LaunchedEffect(wantsOpen) {
        if (wantsOpen) {
            kotlinx.coroutines.delay(400)
            focusedWithoutIme = !imeOpenNow.value
        } else {
            focusedWithoutIme = false
        }
    }
    val pinnedOpen = hold.held || restoring || focusedWithoutIme
    // Short and only for the cases the keyboard is not driving.
    val pinned by animateFloatAsState(
        targetValue = if (pinnedOpen) 1f else 0f,
        animationSpec = tween(durationMillis = 50, easing = FastOutSlowInEasing),
        label = "composerPinned",
    )
    // One value drives every part of the morph (corner, heights, strips), so they
    // can never drift out of step with each other.
    val expansion = maxOf(pinned, imeExpansion(imeBottomPx, imeRef[0]))

    // Back (or any keyboard dismissal) hides the IME without taking focus from the
    // field, so focus alone would leave the field "wanting" the composer open.
    // Reset intent on the visible -> hidden transition only: right after a tap the
    // keyboard is still on its way up and also reads as hidden, which must not
    // count as a close. A held composer (sheet/picker) hides it on purpose.
    var imeWasVisible by remember { mutableStateOf(false) }
    LaunchedEffect(imeOpen) {
        if (imeWasVisible && !imeOpen && !hold.held) {
            wantsOpen = false
            focusManager.clearFocus()
        }
        imeWasVisible = imeOpen
    }
    val onFieldFocusChange: (Boolean) -> Unit = { hasFocus ->
        if (!hold.held) wantsOpen = hasFocus
    }
    // Whatever was holding the composer open has closed; if the user was typing,
    // hand focus back so they are typing again rather than facing a stale pill.
    LaunchedEffect(hold.held) {
        if (!hold.held && wantsOpen) {
            restoring = true
            try {
                focusField()
                // Bridges the gap until the keyboard's own animation takes over.
                kotlinx.coroutines.delay(500)
            } finally {
                restoring = false
            }
        }
    }
    // Sending is the end of composing: fold back to the pill so the reply has room.
    val sendAndCollapse = {
        wantsOpen = false
        focusManager.clearFocus()
        onSend()
    }

    // ime minus nav bars: Scaffold already pads the nav bar, so only lift
    // by the keyboard itself — otherwise the gap doubles when typing.
    CompositionLocalProvider(LocalComposerHold provides hold) {
    Column(modifier = Modifier.fillMaxWidth().background(NewTheme.Background).windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)).padding(horizontal = 10.dp).padding(top = 8.dp, bottom = 8.dp)) {
        if (topBanner != null) topBanner()
        // Kept in composition while collapsed (just zero-height) so a sheet it owns
        // and its loaded branch list survive the composer folding up.
        Collapsible(progress = expansion, anchorBottom = true, modifier = Modifier.fillMaxWidth()) {
            ComposerTopStrip(sessionId = sessionId, running = running, projectLocked = projectLocked, onAddProject = onAddProject)
        }
        // Attachments are content, not configuration, so they stay visible collapsed.
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
            onSend = sendAndCollapse,
            onStop = onStop,
            expansion = expansion,
            focusRequester = focusRequester,
            onFocusChange = onFieldFocusChange,
            onTapBubble = ::focusField,
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
        Collapsible(progress = expansion, modifier = Modifier.fillMaxWidth()) {
            ComposerBottomStrip(sessionId = sessionId)
        }
    }
    }
}

private val INPUT_LINE_HEIGHT = 19.sp

/** The input bubble: one-line pill when idle, full composer when focused. */
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
    /** 0 = collapsed pill, 1 = fully expanded. */
    expansion: Float,
    focusRequester: FocusRequester,
    onFocusChange: (Boolean) -> Unit,
    onTapBubble: () -> Unit,
) {
    val density = androidx.compose.ui.platform.LocalDensity.current
    // Derived from the text's own line height so everything tracks the system font size.
    val lineDp = with(density) { INPUT_LINE_HEIGHT.toDp() }
    // Collapsed, the field row is exactly as tall as the send button so the pill
    // sits level with it; the padding that centres the line shrinks to the
    // normal 6dp as it expands.
    val collapsedPad = ((SEND_SIZE - lineDp) / 2).coerceAtLeast(6.dp)
    val fieldVPad = androidx.compose.ui.unit.lerp(collapsedPad, 6.dp, expansion)
    // One line when collapsed; two lines of text (three with the button row) when
    // expanded, growing to the same maximum as before as you type.
    val fieldMinHeight = androidx.compose.ui.unit.lerp(lineDp, lineDp * 2, expansion)
    val fieldMaxHeight = androidx.compose.ui.unit.lerp(lineDp, maxOf(80.dp, lineDp * 2), expansion)
    // Collapsed, the send button sits at the end of the same row, so text must stop short of it.
    val fieldEndPad = androidx.compose.ui.unit.lerp(SEND_SIZE + 4.dp, 8.dp, expansion)
    // Pill when collapsed, the rounded rect when expanded; radii larger than half
    // the height are clamped, so 26dp reads as a full pill at any font scale.
    val bubbleShape = RoundedCornerShape(androidx.compose.ui.unit.lerp(26.dp, 20.dp, expansion))
    Box(
        modifier = Modifier.fillMaxWidth().clip(bubbleShape)
            .background(NewTheme.Card)
            .onGloballyPositioned(onCoordinatesChange)
            // The whole pill is the tap target, not just the sliver of text.
            .pointerInput(Unit) { detectTapGestures { onTapBubble() } }
            .padding(horizontal = 6.dp, vertical = 6.dp),
    ) {
        Column {
        var mentionLayout by remember { mutableStateOf<androidx.compose.ui.text.TextLayoutResult?>(null) }
        var fieldHeightPx by remember { mutableStateOf(0) }
        // The editable field cannot host icon glyphs, so mention icons ride
        // as an overlay on each collapsed mention's reserved slot. Hidden
        // while the field scrolls internally (layout taller than the box),
        // where slot coordinates would no longer line up.
        Box(
            modifier = Modifier
                .fillMaxWidth()
                .padding(start = 8.dp, end = fieldEndPad, top = fieldVPad, bottom = fieldVPad)
                .heightIn(min = fieldMinHeight, max = fieldMaxHeight)
                .onGloballyPositioned { fieldHeightPx = it.size.height }
                .clipToBounds(),
        ) {
            BasicTextField(
                value = fieldValue,
                onValueChange = onFieldValueChange,
                modifier = Modifier.fillMaxWidth().focusRequester(focusRequester).onFocusChanged { onFocusChange(it.isFocused) },
                textStyle = androidx.compose.ui.text.TextStyle(
                    color = NewTheme.TextPrimary,
                    fontSize = 14.sp,
                    lineHeight = INPUT_LINE_HEIGHT,
                ),
                cursorBrush = SolidColor(NewTheme.Accent),
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
                            Text("Ask anything…", color = NewTheme.TextMuted, fontSize = 14.sp)
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
            // Attach and the chips only exist once expanded; the send button below is
            // the one control that is always there, so it lives outside this row.
            Collapsible(progress = expansion, modifier = Modifier.fillMaxWidth()) {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(top = 2.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(
                        modifier = Modifier.size(SEND_SIZE).clip(CircleShape).clickable(onClickLabel = "Attach image", onClick = attach),
                        contentAlignment = Alignment.Center,
                    ) {
                        androidx.compose.material3.Icon(TablerIcons.Outline.Plus, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(20.dp))
                    }
                    // Fills the gap so the chips stay clear of the send button; the model
                    // chip truncates rather than pushing anything off a narrow screen.
                    Row(
                        modifier = Modifier.weight(1f).padding(horizontal = 4.dp),
                        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(6.dp, Alignment.End),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        ModelChip(sessionId = sessionId, modifier = Modifier.weight(1f, fill = false))
                        ThinkingChip(sessionId = sessionId, running = running)
                    }
                    // Reserves the slot the send button floats over.
                    Spacer(Modifier.size(SEND_SIZE))
                }
            }
        }
        // Pinned bottom-end: beside the text when collapsed, at the end of the
        // action row when expanded. The bubble's height animates and the button
        // rides its bottom edge, so it slides into place with no separate animation.
        SendStopButton(
            running = running,
            canSend = canSend,
            onSend = onSend,
            onStop = onStop,
            modifier = Modifier.align(Alignment.BottomEnd),
        )
    }
}

private val SEND_SIZE = 35.dp

@Composable
private fun SendStopButton(
    running: Boolean,
    canSend: Boolean,
    onSend: () -> Unit,
    onStop: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (running) {
        // Same footprint as the send button so the composer doesn't resize
        // mid-send, and destructive red so the control's meaning is readable at
        // a glance rather than only from a tiny glyph.
        Box(
            modifier = modifier.size(SEND_SIZE).clip(CircleShape).background(NewTheme.Danger)
                .clickable(onClickLabel = "Stop", onClick = onStop),
            contentAlignment = Alignment.Center,
        ) {
            androidx.compose.material3.Icon(TablerIcons.Outline.PlayerStop, contentDescription = "Stop generating", tint = NewTheme.OnPrimary, modifier = Modifier.size(14.dp))
        }
    } else {
        Box(
            modifier = modifier.size(SEND_SIZE).clip(CircleShape)
                .background(if (canSend) NewTheme.Primary else NewTheme.PrimaryDisabled)
                .clickable(enabled = canSend, onClickLabel = "Send", onClick = onSend),
            contentAlignment = Alignment.Center,
        ) {
            androidx.compose.material3.Icon(TablerIcons.Outline.Send, contentDescription = null, tint = if (canSend) NewTheme.OnPrimary else NewTheme.TextMuted, modifier = Modifier.size(15.dp))
        }
    }
}
