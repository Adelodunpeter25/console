package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Lock
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatModelName
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.picker.ApprovalModePickerSheet
import com.console.mobile.ui.components.picker.ModelPickerSheet
import com.console.mobile.ui.components.picker.PickerChip
import com.console.mobile.ui.components.picker.ProjectPickerSheet
import kotlinx.coroutines.launch

/**
 * The project / model / approval chips under the input, and the sheets they
 * open. Owns the session mutations the selections trigger — the input field
 * itself never writes to the session.
 */
@Composable
fun ComposerBottomStrip(sessionId: String, projectLocked: Boolean) {
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    val projects = projectState.projects
    var projectSheet by remember { mutableStateOf(false) }
    var modelSheet by remember { mutableStateOf(false) }
    var approvalSheet by remember { mutableStateOf(false) }

    fun updateSession(dto: UpdateSessionDto) {
        scope.launch {
            AppContainer.projectRepository.updateSession(sessionId, dto)
            AppContainer.sessionRepository.refreshHeader(sessionId)
        }
    }

    val selectedProject = projects.firstOrNull { p ->
        (view?.sessionCwd?.isNotEmpty() == true && (p.path == view.sessionCwd || view.sessionCwd.startsWith(p.path + "/"))) || p.id == null
    } ?: projects.firstOrNull { it.path == view?.sessionCwd }
    val modelLabel = view?.sessionModelId?.ifBlank { null }?.let { formatModelName(it) } ?: "Default Model"
    val modeLabel = when (ApprovalMode.fromValue(view?.approvalMode ?: "")) {
        ApprovalMode.AlwaysAsk -> "Always Ask"
        ApprovalMode.AcceptEdits -> "Accept Edits"
        ApprovalMode.PlanMode -> "Plan Mode"
        ApprovalMode.FullAccess -> "Full Access"
    }

    LazyRow(modifier = Modifier.fillMaxWidth().padding(top = 8.dp, start = 6.dp, end = 6.dp, bottom = 4.dp)) {
        item {
            PickerChip(
                icon = if (projectLocked) TablerIcons.Outline.Lock else TablerIcons.Outline.FolderOpen,
                label = selectedProject?.name ?: "Select Folder",
                modifier = Modifier.padding(end = 8.dp),
            ) {
                if (!projectLocked) {
                    if (projects.isEmpty()) AppContainer.projectRepository.loadProjects()
                    projectSheet = true
                }
            }
        }
        item {
            PickerChip(
                icon = Icons.Filled.SmartToy,
                label = modelLabel,
                provider = view?.sessionProvider,
                modifier = Modifier.padding(end = 8.dp),
            ) {
                AppContainer.providerRepository.loadProviders()
                modelSheet = true
            }
        }
        item {
            PickerChip(icon = Icons.Filled.Shield, label = modeLabel) { approvalSheet = true }
        }
    }

    if (projectSheet) {
        ProjectPickerSheet(
            projects = projects,
            selectedId = selectedProject?.id,
            locked = projectLocked,
            onDismiss = { projectSheet = false },
            onSelect = { proj ->
                projectSheet = false
                updateSession(UpdateSessionDto(cwd = proj.path))
            },
        )
    }
    if (modelSheet) {
        ModelPickerSheet(
            selectedModel = view?.sessionModelId,
            selectedProvider = view?.sessionProvider,
            onDismiss = { modelSheet = false },
            onSelect = { modelId, provider ->
                modelSheet = false
                updateSession(UpdateSessionDto(modelId = modelId, provider = provider))
            },
        )
    }
    if (approvalSheet) {
        ApprovalModePickerSheet(
            current = view?.approvalMode ?: ApprovalMode.AlwaysAsk.value,
            onDismiss = { approvalSheet = false },
            onSelect = { mode ->
                approvalSheet = false
                updateSession(UpdateSessionDto(approvalMode = mode))
            },
        )
    }
}
