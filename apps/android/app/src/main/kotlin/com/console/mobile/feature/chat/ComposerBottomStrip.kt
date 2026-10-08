package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
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
import com.console.mobile.ui.components.picker.PickerChip
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Brain
import io.github.lyxnx.compose.ui.tablericons.outline.Robot
import io.github.lyxnx.compose.ui.tablericons.outline.Shield
import kotlinx.coroutines.launch

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
 * Tap-to-cycle thinking chip, mirroring desktop's stepper. The levels come from
 * the model the backend reports; with none declared the chip is not shown.
 * Disabled during a run, where a change would only apply to the next turn.
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
    val step = com.console.mobile.core.chat.thinkingStepLabel(supported, current) ?: return

    PickerChip(
        icon = TablerIcons.Outline.Brain,
        label = "$step ${current.replaceFirstChar { it.uppercase() }}",
        enabled = !running,
        modifier = modifier,
    ) {
        val next = com.console.mobile.core.chat.nextThinkingLevel(supported, current) ?: return@PickerChip
        scope.launch {
            AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(thinkingLevel = next))
            AppContainer.sessionRepository.refreshHeader(sessionId)
        }
    }
}
