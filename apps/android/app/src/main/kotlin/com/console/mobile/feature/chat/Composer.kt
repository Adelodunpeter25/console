package com.console.mobile.feature.chat
import io.github.lyxnx.compose.ui.tablericons.outline.MessageChatbot

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
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import kotlinx.coroutines.launch
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.clipToBounds
import androidx.compose.ui.draw.drawBehind
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
import com.console.mobile.core.util.filterChatRefs
import com.console.mobile.core.util.parseFileMentions
import com.console.mobile.core.util.slugifyChatTitle
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import console.v1.FileSearchResult
import console.v1.SessionHeader
import console.v1.SlashCommandInfo
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.common.new.BlurredBottomBar
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Microphone
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerStop
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowUp
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
    onConnectGithub: () -> Unit = {},
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
    // Speech-to-text dictation. Enabled only when the server has a
    // credential; the status loads at app start, never only from Providers.
    val authState by AppContainer.authStateHolder.state.collectAsStateWithLifecycle()
    val dictationEnabled = authState.sttStatus?.configured == true
    var dictationRecording by remember(sessionId) { mutableStateOf(false) }
    var dictationTranscribing by remember(sessionId) { mutableStateOf(false) }
    var dictationElapsedSecs by remember(sessionId) { mutableStateOf(0) }
    var dictationError by remember(sessionId) { mutableStateOf<String?>(null) }
    val context = androidx.compose.ui.platform.LocalContext.current
    val scope = rememberCoroutineScope()
    val recorder = remember(sessionId) { com.console.mobile.core.audio.DictationRecorder(context.applicationContext) }
    androidx.compose.runtime.DisposableEffect(sessionId) {
        onDispose { recorder.cancel() }
    }
    fun startDictation() {
        dictationError = null
        try {
            recorder.start()
            dictationRecording = true
            dictationElapsedSecs = 0
        } catch (e: Exception) {
            dictationError = "Couldn't start recording. ${e.message.orEmpty().trim()} In Settings, allow microphone access for Console."
        }
    }
    fun finishDictation() {
        dictationRecording = false
        val bytes = try {
            recorder.stop()
        } catch (e: Exception) {
            dictationError = e.message ?: "Recording failed."
            return
        }
        if (bytes.isEmpty()) {
            dictationError = "Nothing was recorded — check the microphone and try again."
            return
        }
        dictationTranscribing = true
        val draft = value
        scope.launch {
            try {
                val text = AppContainer.authRepository.transcribeStt(bytes, "dictation.m4a", "audio/mp4").trim()
                if (text.isEmpty()) {
                    dictationError = "I couldn't hear anything — try again."
                } else {
                    val separator = if (draft.isBlank()) "" else " "
                    onChange((draft + separator + text).trimEnd())
                }
            } catch (e: Exception) {
                dictationError = "Transcription failed: ${e.message.orEmpty().ifBlank { "Couldn't reach the server." }}"
            } finally {
                dictationTranscribing = false
            }
        }
    }
    val micPermission = androidx.activity.compose.rememberLauncherForActivityResult(
        androidx.activity.result.contract.ActivityResultContracts.RequestPermission(),
    ) { granted ->
        if (granted) startDictation()
        else dictationError = "Microphone access denied. Allow it in Settings → Apps → Console → Permissions."
    }
    // The elapsed label ticks while a take runs; at 120s the take ends on its
    // own, since longer audio would exceed the server's 4 MB cap.
    androidx.compose.runtime.LaunchedEffect(dictationRecording) {
        if (!dictationRecording) return@LaunchedEffect
        val startedAt = System.currentTimeMillis()
        while (true) {
            kotlinx.coroutines.delay(250)
            val secs = ((System.currentTimeMillis() - startedAt) / 1000).toInt()
            dictationElapsedSecs = secs
            if (secs >= com.console.mobile.core.audio.MAX_RECORD_SECONDS) {
                finishDictation()
                break
            }
        }
    }

    var fieldValue by remember(sessionId) { mutableStateOf(TextFieldValue(text = value, selection = TextRange(value.length))) }
    if (fieldValue.text != value) {
        fieldValue = fieldValue.copy(text = value, selection = TextRange(minOf(fieldValue.selection.start, value.length)))
    }
    // Paths inserted from the autocomplete popup. Only these count as
    // confirmed chips for atomic backspace — freshly typed `@query` text
    // (even when it parses as a mention) keeps char-by-char deletion.
    var confirmedPaths by remember(sessionId) { mutableStateOf(setOf<String>()) }
    var confirmedSkills by remember(sessionId) { mutableStateOf(setOf<String>()) }
    var confirmedChatRefs by remember(sessionId) { mutableStateOf(setOf<String>()) }
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
    val confirmedSkillMentions = remember(fieldValue.text, trigger, confirmedSkills) {
        val all = com.console.mobile.core.util.parseSkillMentions(fieldValue.text)
            .filter { it.name in confirmedSkills }
        val t = trigger
        if (t is ComposerTrigger.Slash) {
            val cursor = fieldValue.selection.start
            all.filterNot { it.range.first < cursor && it.range.last + 1 > t.start }
        } else {
            all
        }
    }
    val confirmedChatMentions = remember(fieldValue.text, trigger, confirmedChatRefs) {
        val all = com.console.mobile.core.util.parseChatMentions(fieldValue.text)
            .filter { it.slug in confirmedChatRefs }
        val t = trigger
        if (t is ComposerTrigger.Chat) {
            val cursor = fieldValue.selection.start
            all.filterNot { it.range.first < cursor && it.range.last + 1 > t.start }
        } else {
            all
        }
    }
    val mentionVisual = remember(fieldValue.text, confirmedMentions, confirmedSkillMentions, confirmedChatMentions) {
        buildMentionVisual(fieldValue.text, confirmedMentions, confirmedSkillMentions, confirmedChatMentions)
    }

    var slashCommands by remember(sessionId) { mutableStateOf<List<SlashCommandInfo>>(emptyList()) }
    LaunchedEffect(sessionId, trigger is ComposerTrigger.Slash) {
        if (trigger is ComposerTrigger.Slash && slashCommands.isEmpty()) {
            slashCommands = AppContainer.assistRepository.listSlashCommands(sessionId)
        }
    }
    var chatRefSessions by remember(sessionId) { mutableStateOf<List<SessionHeader>>(emptyList()) }
    LaunchedEffect(sessionId, trigger is ComposerTrigger.Chat) {
        if (chatRefSessions.isEmpty() || trigger is ComposerTrigger.Chat) {
            chatRefSessions = AppContainer.assistRepository.listChatRefs(sessionId)
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

    fun applyChatSuggestion(session: SessionHeader, replaceFrom: Int) {
        val slug = slugifyChatTitle(session.title ?: "").ifBlank { session.id }
        confirmedChatRefs = confirmedChatRefs + slug
        applySuggestion("\$$slug ", replaceFrom)
    }

    fun applyFileSuggestion(file: console.v1.FileSearchResult, replaceFrom: Int) {
        val path = if (file.is_dir && !file.relative_path.endsWith('/')) "${file.relative_path}/" else file.relative_path
        confirmedPaths = confirmedPaths + path
        applySuggestion("@$path ", replaceFrom)
    }

    fun applySkillSuggestion(command: SlashCommandInfo, replaceFrom: Int) {
        if (!command.builtin) {
            confirmedSkills = confirmedSkills + command.name
        }
        applySuggestion("/${command.name} ", replaceFrom)
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
    BlurredBottomBar(
        modifier = Modifier
            .windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)),
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 10.dp)
                .padding(top = 8.dp, bottom = 8.dp),
        ) {
            if (topBanner != null) topBanner()
            // Kept in composition while collapsed (just zero-height) so a sheet it owns
            // and its loaded branch list survive the composer folding up.
            // Project and branch are only chosen before the first message, so the strip is gone after.
            if (!projectLocked) Collapsible(progress = expansion, anchorBottom = true, modifier = Modifier.fillMaxWidth()) {
                ComposerTopStrip(sessionId = sessionId, running = running, projectLocked = projectLocked, onAddProject = onAddProject, onConnectGithub = onConnectGithub)
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
                        val skills = com.console.mobile.core.util.parseSkillMentions(old.text)
                        var targetStart: Int? = null
                        var targetEnd: Int? = null

                        val fileTarget = mentions.firstOrNull { deletedAt in it.range && it.path in confirmedPaths }
                            ?: if (old.text[deletedAt].isWhitespace()) mentions.firstOrNull { it.range.last + 1 == deletedAt && it.path in confirmedPaths } else null
                        if (fileTarget != null) {
                            targetStart = fileTarget.range.first
                            targetEnd = if (old.text[deletedAt].isWhitespace() && fileTarget.range.last + 1 == deletedAt) deletedAt + 1 else fileTarget.range.last + 1
                        } else {
                            val skillTarget = skills.firstOrNull { deletedAt in it.range && it.name in confirmedSkills }
                                ?: if (old.text[deletedAt].isWhitespace()) skills.firstOrNull { it.range.last + 1 == deletedAt && it.name in confirmedSkills } else null
                            if (skillTarget != null) {
                                targetStart = skillTarget.range.first
                                targetEnd = if (old.text[deletedAt].isWhitespace() && skillTarget.range.last + 1 == deletedAt) deletedAt + 1 else skillTarget.range.last + 1
                            }
                        }

                        if (targetStart != null && targetEnd != null) {
                            nextText = old.text.removeRange(targetStart, targetEnd)
                            nextSelection = TextRange(targetStart)
                        }
                    }
                }
                // Prune confirmations whose text is gone.
                confirmedPaths = confirmedPaths.filter { "@$it" in nextText }.toSet()
                confirmedSkills = confirmedSkills.filter { "/$it" in nextText }.toSet()
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
            dictationEnabled = dictationEnabled,
            dictationRecording = dictationRecording,
            dictationTranscribing = dictationTranscribing,
            dictationElapsedSecs = dictationElapsedSecs,
            onMicClick = {
                if (dictationRecording) finishDictation()
                else if (!dictationTranscribing) micPermission.launch(android.Manifest.permission.RECORD_AUDIO)
            },
        )
        if (dictationError != null) {
            Text(
                dictationError!!,
                color = NewTheme.Danger,
                fontSize = 12.sp,
                modifier = Modifier.padding(horizontal = 6.dp, vertical = 4.dp),
            )
        }
        val anchor = fieldCoordinates
        if (anchor != null) {
            when (val t = trigger) {
                is ComposerTrigger.Slash -> {
                    val items = slashCommands.filter { it.name.contains(t.query, ignoreCase = true) }
                    if (items.isNotEmpty()) {
                        ComposerAutocompletePopup(anchor = anchor) {
                            items.take(20).forEach { cmd ->
                                SlashCommandSuggestionRow(command = cmd) {
                                    applySkillSuggestion(cmd, t.start)
                                }
                            }
                        }
                    }
                }
                is ComposerTrigger.Chat -> {
                    val items = filterChatRefs(chatRefSessions, sessionId, t.query)
                    if (items.isNotEmpty()) {
                        ComposerAutocompletePopup(anchor = anchor) {
                            items.take(20).forEach { session ->
                                ChatRefSuggestionRow(session = session) {
                                    applyChatSuggestion(session, t.start)
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
    // Speech-to-text dictation (see DictationButton); owned by Composer.
    dictationEnabled: Boolean,
    dictationRecording: Boolean,
    dictationTranscribing: Boolean,
    dictationElapsedSecs: Int,
    onMicClick: () -> Unit,
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
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val planOn = com.console.mobile.data.model.ApprovalMode.fromValue(sessionViews[sessionId]?.approvalMode) ==
        com.console.mobile.data.model.ApprovalMode.PlanMode
    Box(
        modifier = Modifier.fillMaxWidth().clip(bubbleShape)
            .background(NewTheme.Card)
            // Plan mode reads at a glance: faint diagonal stripes over the bubble.
            .then(
                if (planOn) Modifier.drawBehind {
                    val step = 11.dp.toPx()
                    val stroke = 1.2.dp.toPx()
                    var x = -size.height
                    while (x < size.width) {
                        drawLine(
                            color = NewTheme.Accent.copy(alpha = 0.14f),
                            start = androidx.compose.ui.geometry.Offset(x, size.height),
                            end = androidx.compose.ui.geometry.Offset(x + size.height, 0f),
                            strokeWidth = stroke,
                        )
                        x += step
                    }
                } else Modifier,
            )
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
                    Box(contentAlignment = Alignment.TopStart) {
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
                                            .align(Alignment.TopStart)
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
                            if (seg.kind == MentionKind.Skill) {
                                com.console.mobile.ui.components.SkillIcon(sizeDp = 12)
                            } else if (seg.kind == MentionKind.Chat) {
                                androidx.compose.material3.Icon(
                                    TablerIcons.Outline.MessageChatbot,
                                    contentDescription = null,
                                    tint = MentionAccent,
                                )
                            } else if (seg.path.endsWith('/')) {
                                androidx.compose.material3.Icon(
                                    TablerIcons.Outline.Folder,
                                    contentDescription = null,
                                    tint = MentionAccent,
                                    modifier = Modifier.size(12.dp),
                                )
                            } else {
                                FileIcon(filename = seg.path.substringAfterLast('/'), sizeDp = 12)
                            }
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
                    // Fills the gap so the chips stay clear of the pinned mic and
                    // send buttons; the model chip truncates rather than pushing
                    // anything off a narrow screen.
                    ThinkingChip(sessionId = sessionId, running = running)
                    Row(
                        modifier = Modifier.weight(1f).padding(horizontal = 4.dp),
                        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(6.dp, Alignment.End),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        ModelChip(sessionId = sessionId, modifier = Modifier.weight(1f, fill = false))
                    }
                    // Reserves the mic + send slots the pinned controls float
                    // over, so the model picker moves aside instead of being
                    // covered. The mic zone widens while recording so the
                    // timer label never reaches the picker either.
                    val micSlot = SEND_SIZE + 8.dp + if (dictationRecording) 30.dp else 0.dp
                    Spacer(Modifier.size(width = micSlot, height = SEND_SIZE))
                    // Reserves the slot the send button floats over.
                    Spacer(Modifier.size(SEND_SIZE))
                }
            }
        }
        // Pinned bottom-end: beside the text when collapsed, at the end of the
        // action row when expanded. The bubble's height animates and the button
        // rides its bottom edge, so it slides into place with no separate animation.
        // The mic pins just left of send in BOTH states — never floating over
        // the model picker (which is pushed aside by the row's reservations).
        DictationButton(
                enabled = dictationEnabled,
                recording = dictationRecording,
                transcribing = dictationTranscribing,
                elapsedSecs = dictationElapsedSecs,
                onMicClick = onMicClick,
                modifier = Modifier.align(Alignment.BottomEnd)
                    .offset(x = -(SEND_SIZE + 8.dp)),
        )
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

/**
 * Voice dictation control, pinned just left of the send button. Idle: the
 * mic starts a take. Recording: the red stop control takes its place with
 * the live elapsed label. Unconfigured: dimmed and inert.
 */
@Composable
private fun DictationButton(
    enabled: Boolean,
    recording: Boolean,
    transcribing: Boolean,
    elapsedSecs: Int,
    onMicClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier,
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(6.dp, Alignment.End),
    ) {
        if (recording) {
            Text(
                text = "%d:%02d".format(elapsedSecs / 60, elapsedSecs % 60),
                color = NewTheme.Danger,
                fontSize = 12.sp,
                fontWeight = androidx.compose.ui.text.font.FontWeight.Medium,
                modifier = Modifier.padding(end = 2.dp),
            )
        }
        when {
            recording -> {
                Box(
                    modifier = Modifier.size(SEND_SIZE).clip(CircleShape).background(NewTheme.Danger)
                        .clickable(onClickLabel = "Stop recording", onClick = onMicClick),
                    contentAlignment = Alignment.Center,
                ) {
                    androidx.compose.material3.Icon(
                        TablerIcons.Outline.PlayerStop,
                        contentDescription = "Stop recording",
                        tint = NewTheme.OnPrimary,
                        modifier = Modifier.size(14.dp),
                    )
                }
            }
            transcribing -> {
                Box(
                    modifier = Modifier.size(SEND_SIZE),
                    contentAlignment = Alignment.Center,
                ) {
                    androidx.compose.material3.CircularProgressIndicator(
                        color = NewTheme.TextSecondary,
                        modifier = Modifier.size(16.dp),
                        strokeWidth = 2.dp,
                    )
                }
            }
            !enabled -> {
                // Dim and inert until the server has a credential: a button
                // that fails on tap is worse than a visibly disabled one.
                Box(
                    modifier = Modifier.size(SEND_SIZE).clip(CircleShape)
                        .background(NewTheme.Card),
                    contentAlignment = Alignment.Center,
                ) {
                    androidx.compose.material3.Icon(
                        TablerIcons.Outline.Microphone,
                        contentDescription = "Set up Speech to Text in Settings",
                        tint = NewTheme.TextMuted.copy(alpha = 0.4f),
                        modifier = Modifier.size(16.dp),
                    )
                }
            }
            else -> {
                Box(
                    modifier = Modifier.size(SEND_SIZE).clip(CircleShape).background(NewTheme.Card)
                        .clickable(onClickLabel = "Record dictation", onClick = onMicClick),
                    contentAlignment = Alignment.Center,
                ) {
                    androidx.compose.material3.Icon(
                        TablerIcons.Outline.Microphone,
                        contentDescription = "Record dictation",
                        tint = NewTheme.TextSecondary,
                        modifier = Modifier.size(16.dp),
                    )
                }
            }
        }
    }
}

@Composable
private fun SendStopButton(
    running: Boolean,
    canSend: Boolean,
    onSend: () -> Unit,
    onStop: () -> Unit,
    modifier: Modifier = Modifier,
) {
    // While a run is active, typed text sends (it queues for the next turn);
    // Stop shows only when there is nothing to send.
    if (running && !canSend) {
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
            androidx.compose.material3.Icon(TablerIcons.Outline.ArrowUp, contentDescription = null, tint = if (canSend) NewTheme.OnPrimary else NewTheme.TextMuted, modifier = Modifier.size(15.dp))
        }
    }
}
