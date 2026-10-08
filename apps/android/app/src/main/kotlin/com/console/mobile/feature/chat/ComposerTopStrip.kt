package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.chat.projectForSession
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.picker.BranchPickerSheet
import com.console.mobile.ui.components.picker.PickerChip
import com.console.mobile.ui.components.picker.ProjectPickerSheet
import console.v1.GitBranchInfo
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.GitBranch
import io.github.lyxnx.compose.ui.tablericons.outline.Lock
import kotlinx.coroutines.launch

/**
 * Project → branch bubbles above the input. Branch is scoped to the project, so
 * it reloads when the project changes and disappears for a folder with no git
 * repo rather than showing a control that can't do anything.
 */
@Composable
fun ComposerTopStrip(sessionId: String, running: Boolean, projectLocked: Boolean, onAddProject: () -> Unit) {
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    val cwd = view?.sessionCwd
    val projects = projectState.projects
    var projectSheet by remember { mutableStateOf(false) }
    var branchSheet by remember { mutableStateOf(false) }

    val selectedProject = projectForSession(projects, view?.projectId, cwd)

    // null = not fetched yet, empty list with isRepo=false = not a git repo.
    var branches by remember(cwd) { mutableStateOf<List<GitBranchInfo>?>(null) }
    var isRepo by remember(cwd) { mutableStateOf<Boolean?>(null) }
    var loadingBranches by remember(cwd) { mutableStateOf(false) }
    // Reloads after a checkout too, so the chip shows the branch actually checked out.
    var reloadTick by remember(cwd) { mutableStateOf(0) }
    // The branch being switched to (sheet stays open, showing progress) and the
    // reason a switch was refused. Both live here so they survive recomposition.
    var switchingTo by remember(cwd) { mutableStateOf<String?>(null) }
    var switchError by remember(cwd) { mutableStateOf<String?>(null) }
    LaunchedEffect(cwd, reloadTick) {
        if (cwd.isNullOrEmpty()) { isRepo = false; branches = null; return@LaunchedEffect }
        loadingBranches = true
        try {
            val res = AppContainer.gitRepository.listBranches(cwd)
            isRepo = res?.is_git_repository == true
            branches = res?.branches
        } catch (_: Exception) {
            // Leave the previous value; the chip just stays as it was.
        } finally {
            loadingBranches = false
        }
    }

    val worktreeBranch = view?.worktreeBranch
    val currentBranch = worktreeBranch ?: branches?.firstOrNull { it.current }?.name
    val hasMessages = projectLocked

    LazyRow(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp)) {
        item {
            PickerChip(
                icon = if (projectLocked) TablerIcons.Outline.Lock else TablerIcons.Outline.Folder,
                label = selectedProject?.name ?: if (cwd.isNullOrEmpty()) "Select Folder" else "No project",
                modifier = Modifier.padding(end = 8.dp),
            ) {
                if (!projectLocked) {
                    if (projects.isEmpty()) AppContainer.projectRepository.loadProjects()
                    projectSheet = true
                }
            }
        }
        if (isRepo == true && currentBranch != null) {
            item {
                PickerChip(icon = TablerIcons.Outline.GitBranch, label = currentBranch) { branchSheet = true }
            }
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
                // The branch belongs to the old project; drop it so the chip
                // can't show a stale name while the new project's loads.
                branches = null
                isRepo = null
                scope.launch {
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(cwd = proj.path))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
            onSelectNone = {
                projectSheet = false
                branches = null
                isRepo = null
                scope.launch {
                    AppContainer.projectRepository.updateSession(sessionId, UpdateSessionDto(clearProject = true))
                    AppContainer.sessionRepository.refreshHeader(sessionId)
                }
            },
            onAddProject = {
                projectSheet = false
                onAddProject()
            },
        )
    }
    if (branchSheet && cwd != null) {
        // A worktree chat is pinned to its own branch, and a run in flight would
        // only pick a change up on the next turn — neither can be switched here.
        // Starting a worktree is further limited to a chat with no messages yet.
        val reason = when {
            running -> "Locked while a run is active — a change would only apply to the next message."
            worktreeBranch != null -> "This chat works in its own worktree branch."
            else -> null
        }
        BranchPickerSheet(
            branches = branches,
            loading = loadingBranches,
            locked = reason != null,
            lockedReason = reason,
            canStartWorktree = !hasMessages,
            worktreeBranch = worktreeBranch,
            switchingTo = switchingTo,
            error = switchError,
            onDismiss = {
                branchSheet = false
                switchError = null
            },
            onSelect = { name ->
                if (switchingTo == null && name != currentBranch) {
                    switchError = null
                    switchingTo = name
                    scope.launch {
                        try {
                            AppContainer.gitRepository.checkoutBranch(cwd, name)
                            // Reflect the switch immediately; the reload below confirms it.
                            branches = branches?.map { it.copy(current = it.name == name) }
                            reloadTick++
                            branchSheet = false
                        } catch (e: Exception) {
                            // Keep the sheet open: git refuses a switch with conflicting
                            // local changes, or to a branch checked out in another worktree,
                            // and the user needs to see why instead of a silent close.
                            switchError = e.message?.takeIf { it.isNotBlank() } ?: "Couldn't switch branch."
                        } finally {
                            switchingTo = null
                        }
                    }
                }
            },
            onNewWorktree = {
                branchSheet = false
                scope.launch {
                    try {
                        val header = AppContainer.sessionRepository.attachWorktree(sessionId)
                        // The Home list reads its own copy of the header; without this it
                        // keeps showing the project's branch until the list reloads.
                        AppContainer.projectStateHolder.patchSession(sessionId, header)
                    } catch (_: Exception) {
                    }
                }
            },
        )
    }
}
