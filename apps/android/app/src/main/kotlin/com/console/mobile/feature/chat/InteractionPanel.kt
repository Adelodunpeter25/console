package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.HelpCircle
import io.github.lyxnx.compose.ui.tablericons.outline.Shield
import io.github.lyxnx.compose.ui.tablericons.outline.ShieldCheck
import io.github.lyxnx.compose.ui.tablericons.outline.ShieldX
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.PendingPermission
import com.console.mobile.core.chat.PendingQuestion
import com.console.mobile.data.model.AskQuestionRequest
import com.console.mobile.data.model.PermissionRequest
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.InlineTextInput
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.relocation.BringIntoViewRequester
import androidx.compose.foundation.relocation.bringIntoViewRequester
import androidx.compose.foundation.layout.heightIn
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import kotlinx.coroutines.delay

/**
 * Port of components/chat/interactions/interaction-panel.tsx + question-panel.tsx.
 * Permission allow/deny card; question wizard (single/multi select + custom text).
 */
@Composable
fun InteractionPanel(sessionId: String, permissions: List<PendingPermission>, questions: List<PendingQuestion>) {
    if (permissions.isEmpty() && questions.isEmpty()) return
    if (permissions.isNotEmpty()) {
        val first = permissions.first()
        PermissionPanel(request = first.request, sessionId = sessionId)
        return
    }
    QuestionWizard(questions = questions.map { it.request }, sessionId = sessionId)
}

/** The raised card both panels sit in. */
@Composable
private fun PanelCard(modifier: Modifier = Modifier, content: @Composable androidx.compose.foundation.layout.ColumnScope.() -> Unit) {
    Column(
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 8.dp)
            .clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card).padding(18.dp),
        content = content,
    )
}

@Composable
private fun PermissionPanel(request: PermissionRequest, sessionId: String) {
    val scope = rememberCoroutineScope()
    var busy by remember(request.requestId) { mutableStateOf<String?>(null) }
    val argsString = request.args?.toString() ?: ""
    PanelCard {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(TablerIcons.Outline.Shield, contentDescription = null, tint = NewTheme.Warning, modifier = Modifier.size(22.dp))
            Text(
                buildString {
                    append(if (request.requiresUpgrade) "Upgrade permission required: " else "Permission required: ")
                    append(request.toolName)
                },
                color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.Medium,
                modifier = Modifier.weight(1f).padding(start = 12.dp),
            )
        }
        if (!request.reason.isNullOrBlank()) {
            Text(request.reason, color = NewTheme.TextSecondary, fontSize = 14.sp, lineHeight = 21.sp, modifier = Modifier.padding(top = 10.dp))
        }
        if (argsString.isNotEmpty()) {
            Box(modifier = Modifier.fillMaxWidth().padding(top = 12.dp).clip(RoundedCornerShape(NewTheme.FieldRadius)).background(NewTheme.Background).padding(12.dp)) {
                Text(argsString.take(1200), color = NewTheme.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, lineHeight = 18.sp, modifier = Modifier.verticalScroll(rememberScrollState()))
            }
        }
        Row(modifier = Modifier.fillMaxWidth().padding(top = 16.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            val approve = { allow: Boolean ->
                busy = if (allow) "allow" else "deny"
                scope.launch {
                    AppContainer.chatRepository.approvePermission(sessionId, request.requestId, allow)
                    busy = null
                }
            }
            ActionButton(
                text = "Deny",
                onClick = { approve(false) },
                kind = ActionButtonKind.Danger,
                icon = TablerIcons.Outline.ShieldX,
                loading = busy == "deny",
                enabled = busy == null,
                surface = NewTheme.Raised,
                modifier = Modifier.weight(1f),
            )
            ActionButton(
                text = if (request.requiresUpgrade) "Allow once" else "Allow",
                onClick = { approve(true) },
                kind = ActionButtonKind.Primary,
                icon = TablerIcons.Outline.ShieldCheck,
                loading = busy == "allow",
                enabled = busy == null,
                modifier = Modifier.weight(1f),
            )
        }
    }
}

@Composable
private fun QuestionWizard(questions: List<AskQuestionRequest>, sessionId: String) {
    var index by remember(questions.firstOrNull()?.requestId) { mutableStateOf(0) }
    var submitting by remember { mutableStateOf(false) }
    val scope = rememberCoroutineScope()
    val current = questions.getOrNull(index) ?: questions.firstOrNull() ?: return
    val isLast = index >= questions.size - 1

    fun submit(answer: kotlinx.serialization.json.JsonElement) {
        submitting = true
        scope.launch {
            AppContainer.chatRepository.answerQuestion(sessionId, current.requestId, answer)
            submitting = false
            if (!isLast) index += 1
        }
    }

    QuestionPanel(
        request = current,
        total = questions.size,
        index = (index + 1).coerceAtMost(questions.size),
        isLast = isLast,
        submitting = submitting,
        onAnswerText = { submit(JsonPrimitive(it)) },
        onAnswerList = { submit(JsonArray(it.map { s -> JsonPrimitive(s) })) },
        onSkip = { submit(buildJsonObject { put("skipped", true) }) },
    )
}

@Composable
private fun QuestionPanel(
    request: AskQuestionRequest,
    total: Int,
    index: Int,
    isLast: Boolean,
    submitting: Boolean,
    onAnswerText: (String) -> Unit,
    onAnswerList: (List<String>) -> Unit,
    onSkip: () -> Unit,
) {
    var selected by remember(request.requestId) { mutableStateOf(setOf<String>()) }
    var custom by remember(request.requestId) { mutableStateOf("") }
    val hasOptions = request.options.isNotEmpty()
    val hasAnswer = custom.trim().isNotEmpty() || selected.isNotEmpty()

    // --- Keyboard avoiding ---------------------------------------------------
    // The panel rides above the IME (inset padding below), but riding up is not
    // enough on its own: a tall panel (many options + the answer box) was simply
    // clipped at the bottom, hiding the text field and the Submit row. So the
    // body scrolls, the buttons stay pinned, the height is capped, and the field
    // is scrolled into view when it gains focus.
    val density = LocalDensity.current
    val imeOpen = WindowInsets.ime.getBottom(density) > 0
    val screenHeight = LocalConfiguration.current.screenHeightDp.dp
    // Leave the transcript some room while reading; with the keyboard up the panel
    // may use everything left, since the keyboard already took its share.
    val maxHeight = if (imeOpen) screenHeight else screenHeight * 0.62f
    val bodyScroll = rememberScrollState()
    val fieldRequester = remember { BringIntoViewRequester() }
    var fieldFocused by remember { mutableStateOf(false) }
    LaunchedEffect(fieldFocused, imeOpen) {
        // The keyboard animates up after focus lands; wait for the insets to settle
        // or the field is scrolled to where the bottom *was*.
        if (fieldFocused && imeOpen) {
            delay(120)
            fieldRequester.bringIntoView()
        }
    }

    Column(modifier = Modifier.heightIn(max = maxHeight).windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars))) {
        PanelCard {
            Column(modifier = Modifier.weight(1f, fill = false).verticalScroll(bodyScroll)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(TablerIcons.Outline.HelpCircle, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(22.dp))
                    Text(request.question, color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 12.dp))
                    if (total > 1) {
                        Text("$index of $total", color = NewTheme.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily)
                    }
                }
                if (hasOptions) {
                    Column(modifier = Modifier.padding(top = 14.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        request.options.forEach { option ->
                            val isSelected = selected.contains(option)
                            val markShape = if (request.isMultiSelect) RoundedCornerShape(6.dp) else CircleShape
                            Row(
                                modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.FieldRadius))
                                    .background(if (isSelected) NewTheme.Accent.copy(alpha = 0.16f) else NewTheme.Raised)
                                    .clickable {
                                        selected = if (request.isMultiSelect) {
                                            if (isSelected) selected - option else selected + option
                                        } else {
                                            setOf(option)
                                        }
                                    }
                                    .padding(horizontal = 14.dp, vertical = 13.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Box(
                                    modifier = Modifier.size(20.dp).clip(markShape)
                                        .background(if (isSelected) NewTheme.Accent else Color.Transparent)
                                        .border(1.5.dp, if (isSelected) NewTheme.Accent else NewTheme.TextMuted, markShape),
                                    contentAlignment = Alignment.Center,
                                ) {
                                    if (isSelected) {
                                        if (request.isMultiSelect) Icon(TablerIcons.Outline.Check, contentDescription = null, tint = NewTheme.OnPrimary, modifier = Modifier.size(13.dp))
                                        else Box(modifier = Modifier.size(7.dp).clip(CircleShape).background(NewTheme.OnPrimary))
                                    }
                                }
                                Text(option, color = if (isSelected) NewTheme.TextPrimary else NewTheme.TextSecondary, fontSize = 16.sp, fontWeight = if (isSelected) FontWeight.Medium else FontWeight.Normal, modifier = Modifier.padding(start = 14.dp))
                            }
                        }
                    }
                }
                InlineTextInput(
                    value = custom,
                    onValueChange = { custom = it },
                    placeholder = if (hasOptions) "Or type your own answer…" else "Type your answer…",
                    modifier = Modifier.padding(top = 12.dp)
                        .bringIntoViewRequester(fieldRequester)
                        .onFocusChanged { fieldFocused = it.isFocused },
                )
            }
            Row(modifier = Modifier.fillMaxWidth().padding(top = 16.dp), horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                if (request.skippable) {
                    ActionButton(text = "Skip", onClick = onSkip, enabled = !submitting, surface = NewTheme.Raised, modifier = Modifier.weight(1f))
                }
                ActionButton(
                    text = if (isLast) (if (total > 1) "Submit all" else "Submit") else "Next",
                    onClick = {
                        if (custom.trim().isNotEmpty()) onAnswerText(custom.trim())
                        else if (selected.isNotEmpty()) {
                            if (request.isMultiSelect) onAnswerList(selected.toList()) else onAnswerText(selected.first())
                        }
                    },
                    kind = ActionButtonKind.Primary,
                    enabled = hasAnswer && !submitting,
                    loading = submitting,
                    modifier = Modifier.weight(1f),
                )
            }
        }
    }
}
