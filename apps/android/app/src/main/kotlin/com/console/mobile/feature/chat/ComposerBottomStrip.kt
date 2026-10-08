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
        ContextRing(snapshot = chatSessions[sessionId]?.context)
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

/**
 * Model + thinking, fused into one chip. Hides the thinking half when the
 * model reports no levels, rather than showing a control that can't be used.
 */
@Composable
fun ModelThinkingChip(sessionId: String, running: Boolean, modifier: Modifier = Modifier) {
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    var sheet by remember { mutableStateOf(false) }

    val modelId = view?.sessionModelId?.ifBlank { null }
    val provider = view?.sessionProvider
    // Make sure the committed model's list is in memory so its levels are known.
    LaunchedEffect(provider) { provider?.let { AppContainer.providerRepository.loadModels(it) } }
    val model = provider?.let { providerState.modelsByProvider[it] }?.firstOrNull { it.id == modelId }
    val supported = model?.supported_thinking_levels.orEmpty()
    val thinkingLabel = com.console.mobile.core.chat.thinkingChipLabel(supported, view?.thinkingLevel, model?.default_thinking_level)

    val modelLabel = modelId?.let { com.console.mobile.core.util.formatModelName(it) } ?: "Default Model"
    PickerChip(
        icon = TablerIcons.Outline.Robot,
        label = if (thinkingLabel != null) "$modelLabel · $thinkingLabel" else modelLabel,
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
                    // The saved level is left alone here; a run only sends it when
                    // the model still lists it (see ProviderRepository.validThinkingLevel).
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(modelId = newModel, provider = newProvider))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
            selectedThinking = view?.thinkingLevel,
            thinkingLocked = running,
            onSelectThinking = { newModel, newProvider, level ->
                sheet = false
                scope.launch {
                    AppContainer.projectRepository.updateSession(
                        sessionId,
                        UpdateSessionDto(modelId = newModel, provider = newProvider, thinkingLevel = level),
                    )
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
        )
    }
}
