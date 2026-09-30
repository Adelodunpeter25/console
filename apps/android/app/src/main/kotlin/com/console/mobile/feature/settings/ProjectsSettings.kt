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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
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
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import com.console.mobile.AppContainer
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Projects",
            onBack = onBack,
            actions = {
                TextButton(onClick = onAddProject) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(TablerIcons.Outline.Plus, contentDescription = null, tint = ConsoleColors.TextPrimary, modifier = Modifier.size(15.dp))
                        Text("Add Folder", color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 4.dp))
                    }
                }
            },
        )
        if (projectState.loading && projectState.projects.isEmpty()) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                    Text("Loading projects…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                }
            }
        } else if (projectState.error != null && projectState.projects.isEmpty()) {
            EmptyState(
                title = "Couldn't load projects",
                description = projectState.error ?: "Failed to load projects.",
                icon = { Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(32.dp)) },
            )
        } else if (projectState.projects.isEmpty()) {
            EmptyState(title = "No project folders", description = "Add a project folder from your host filesystem to start creating sessions.", icon = { Icon(TablerIcons.Outline.FolderOpen, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(32.dp)) })
        } else {
            LazyColumn(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                items(projectState.projects, key = { it.id }) { proj ->
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
                }
            }
        }
    }
}

@Composable
private fun ProjectRow(proj: ProjectInfo, busy: Boolean, onDelete: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Row(
        modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(shape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, shape).padding(14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(modifier = Modifier.size(32.dp).clip(RoundedCornerShape(8.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp)), contentAlignment = Alignment.Center) {
            Icon(TablerIcons.Outline.FolderOpen, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
        }
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp).padding(end = 12.dp)) {
            Text(proj.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(proj.path, color = ConsoleColors.TextSecondary, fontSize = 12.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
        }
        IconButton(onClick = onDelete, enabled = !busy, modifier = Modifier.size(32.dp)) {
            if (busy) CircularProgressIndicator(color = ConsoleColors.Destructive, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
            else Icon(TablerIcons.Outline.Trash, contentDescription = "Remove", tint = ConsoleColors.Destructive, modifier = Modifier.size(14.dp))
        }
    }
}
