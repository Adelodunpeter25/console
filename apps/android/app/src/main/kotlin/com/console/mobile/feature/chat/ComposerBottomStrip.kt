package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.picker.ApprovalModePickerSheet
import com.console.mobile.ui.components.picker.ThinkingLevelPickerSheet
import com.console.mobile.ui.components.picker.PickerChip
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Brain
import io.github.lyxnx.compose.ui.tablericons.outline.Robot
import io.github.lyxnx.compose.ui.tablericons.outline.Shield
import kotlinx.coroutines.launch
import com.console.mobile.ui.theme.NewTheme

/**
 * Approval mode on the left, context ring on the right. Everything else that
 * used to live here moved up (project, branch) or into the composer's action
 * row (model + thinking), which is what keeps approval on screen on a narrow phone.
 */
@Composable
fun ComposerBottomStrip(sessionId: String) {
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    var approvalSheet by remember { mutableStateOf(false) }
    var usageSheet by remember { mutableStateOf(false) }
    HoldComposerOpen(approvalSheet || usageSheet)

    // A value before the first live update, so the ring isn't blank on entry.
    LaunchedEffect(sessionId) { AppContainer.chatRepository.loadContext(sessionId) }

    val modeLabel = when (ApprovalMode.fromValue(view?.approvalMode ?: "")) {
        ApprovalMode.AlwaysAsk -> "Always Ask"
        ApprovalMode.AcceptEdits -> "Accept Edits"
        ApprovalMode.PlanMode -> "Plan Mode"
        ApprovalMode.FullAccess -> "Full Access"
    }

    Row(
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp, start = 6.dp, end = 6.dp, bottom = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
    ) {
        PickerChip(icon = TablerIcons.Outline.Shield, label = modeLabel) { approvalSheet = true }
        ContextRing(snapshot = chatSessions[sessionId]?.context, onClick = { usageSheet = true })
    }

    if (usageSheet) {
        UsageSheet(sessionId = sessionId, onDismiss = { usageSheet = false })
    }

    if (approvalSheet) {
        ApprovalModePickerSheet(
            current = view?.approvalMode ?: ApprovalMode.AlwaysAsk.value,
            onDismiss = { approvalSheet = false },
            onSelect = { mode ->
                approvalSheet = false
                scope.launch {
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(approvalMode = mode))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
        )
    }
}

/** Model chip. Thinking is its own cycle chip beside it, as on desktop. */
@Composable
fun ModelChip(sessionId: String, modifier: Modifier = Modifier) {
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    var sheet by remember { mutableStateOf(false) }
    HoldComposerOpen(sheet)
    val modelId = view?.sessionModelId?.ifBlank { null }
    val provider = view?.sessionProvider

    PickerChip(
        icon = TablerIcons.Outline.Robot,
        label = modelId?.let { com.console.mobile.core.util.formatModelName(it) } ?: "Default Model",
        provider = provider,
        surface = NewTheme.Raised,
        modifier = modifier,
    ) {
        AppContainer.providerRepository.loadProviders()
        sheet = true
    }

    if (sheet) {
        com.console.mobile.ui.components.picker.ModelPickerSheet(
            selectedModel = view?.sessionModelId,
            selectedProvider = provider,
            onDismiss = { sheet = false },
            onSelect = { newModel, newProvider ->
                sheet = false
                scope.launch {
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(modelId = newModel, provider = newProvider))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
        )
    }
}

/**
 * Brain icon-only button, colored per level like desktop's stepper. Tap opens
 * a level picker sheet instead of cycling. Hidden when the model declares no
 * levels. Disabled during a run, where a change would only apply to the next turn.
 */
@Composable
fun ThinkingChip(sessionId: String, running: Boolean, modifier: Modifier = Modifier) {
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    val modelId = view?.sessionModelId?.ifBlank { null }
    val provider = view?.sessionProvider

    // The committed model's list has to be in memory for its levels to be known.
    LaunchedEffect(provider) { provider?.let { AppContainer.providerRepository.loadModels(it) } }
    val model = provider?.let { providerState.modelsByProvider[it] }?.firstOrNull { it.id == modelId }
        ?: providerState.providers.firstOrNull { it.name == provider }?.models?.firstOrNull { it.id == modelId }
    val supported = model?.supported_thinking_levels.orEmpty()
    val current = com.console.mobile.core.chat.effectiveThinkingLevel(supported, view?.thinkingLevel, model?.default_thinking_level) ?: return
    var sheet by remember { mutableStateOf(false) }
    HoldComposerOpen(sheet)

    Row(
        modifier = modifier.alpha(if (running) 0.45f else 1f)
            .clip(RoundedCornerShape(NewTheme.ChipRadius))
            .background(NewTheme.Raised)
            .clickable(enabled = !running, onClick = { sheet = true })
            .padding(horizontal = 9.dp, vertical = 7.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            TablerIcons.Outline.Brain,
            contentDescription = "Thinking level: $current",
            tint = Color(com.console.mobile.core.chat.thinkingLevelColorHex(current)),
            modifier = Modifier.size(14.dp),
        )
    }

    if (sheet) {
        ThinkingLevelPickerSheet(
            levels = supported,
            current = current,
            onDismiss = { sheet = false },
            onSelect = { level ->
                sheet = false
                scope.launch {
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(thinkingLevel = level))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
        )
    }
}
