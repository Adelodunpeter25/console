package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import com.console.mobile.AppContainer
import console.v1.ProjectInfo
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.AddButton
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider

/**
 * Port of screens/settings/projects-settings.tsx + screens/projects/add-project-screen.tsx.
 * List with remove; "Add Folder" opens the full-screen folder picker.
 */
@Composable
fun ProjectsSettings(onBack: () -> Unit, onAddProject: () -> Unit) {
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    var busyId by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) { AppContainer.projectRepository.loadProjects() }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        ScreenHeader(
            title = "Projects",
            onBack = onBack,
            actions = { AddButton("Add folder", onAddProject) },
        )
        if (projectState.loading && projectState.projects.isEmpty()) {
            LoadingState("Loading projects…", Modifier.fillMaxSize())
        } else if (projectState.error != null && projectState.projects.isEmpty()) {
            EmptyState(
                title = "Couldn't load projects",
                description = projectState.error ?: "Failed to load projects.",
                icon = { Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(32.dp)) },
            )
        } else if (projectState.projects.isEmpty()) {
            EmptyState(title = "No project folders", description = "Add a project folder from your host filesystem to start creating sessions.", icon = { Icon(TablerIcons.Outline.FolderOpen, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(32.dp)) })
        } else {
            Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
                val n = projectState.projects.size
                Section("$n project folder${if (n == 1) "" else "s"}") {
                    projectState.projects.forEachIndexed { index, proj ->
                        ProjectRow(proj = proj, busy = busyId == proj.id, onDelete = {
                            confirmAlert("Remove Project", "Are you sure you want to remove \"${proj.name}\" from your project workspace list? The folder on disk will not be deleted.", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Remove", destructive = true, onPress = {
                                busyId = proj.id
                                scope.launch {
                                    try {
                                        withContext(Dispatchers.IO) { AppContainer.projectRepository.deleteProject(proj.id) }
                                    } catch (e: Exception) {
                                        confirmAlert("Failed", e.message ?: "Unable to remove project.")
                                    } finally { busyId = null }
                                }
                            })))
                        })
                        if (index < projectState.projects.lastIndex) SectionDivider(startInset = 58.dp)
                    }
                }
            }
        }
    }
}

@Composable
private fun ProjectRow(proj: ProjectInfo, busy: Boolean, onDelete: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(start = 18.dp, end = 8.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.Folder, contentDescription = null, tint = NewTheme.TextPrimary, modifier = Modifier.size(24.dp))
        Column(modifier = Modifier.weight(1f).padding(start = 16.dp, end = 8.dp)) {
            Text(proj.name, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(proj.path, color = NewTheme.TextMuted, fontSize = 13.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
        }
        IconButton(onClick = onDelete, enabled = !busy, modifier = Modifier.size(40.dp)) {
            if (busy) CircularProgressIndicator(color = NewTheme.Danger, strokeWidth = 2.dp, modifier = Modifier.size(16.dp))
            else Icon(TablerIcons.Outline.Trash, contentDescription = "Remove", tint = NewTheme.Danger, modifier = Modifier.size(20.dp))
        }
    }
}
