package com.console.mobile.feature.settings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
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
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Eye
import io.github.lyxnx.compose.ui.tablericons.outline.EyeOff
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.PageHeader

/**
 * Full-screen folder picker for adding a project. Starts at the server's home
 * directory and lets the user drill in by tapping — no path typing.
 * The footer pins "Use this folder" for whatever is currently open.
 */
@Composable
fun AddProjectScreen(onBack: () -> Unit, onAdded: () -> Unit) {
    val scope = rememberCoroutineScope()
    var currentPath by remember { mutableStateOf<String?>(null) }
    var parentPath by remember { mutableStateOf<String?>(null) }
    var dirs by remember { mutableStateOf<List<console.v1.FsTreeEntry>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var adding by remember { mutableStateOf(false) }
    var showHidden by remember { mutableStateOf(false) }

    fun browse(path: String?, hidden: Boolean = showHidden) {
        loading = true
        error = null
        scope.launch {
            try {
                val res = withContext(Dispatchers.IO) { AppContainer.fsRepository.browseDirectory(path, hidden) }
                currentPath = res.current_path
                parentPath = res.parent_path
                dirs = res.entries.filter { it.is_dir }
            } catch (e: Exception) {
                error = e.message ?: "Couldn't list that folder."
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(Unit) { browse(null) }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background)) {
        PageHeader(
            title = "Add Project",
            subtitle = currentPath,
            onBack = onBack,
            actions = {
                IconButton(
                    onClick = {
                        val next = !showHidden
                        showHidden = next
                        browse(currentPath, next)
                    },
                    modifier = Modifier.size(40.dp),
                ) {
                    Icon(
                        imageVector = if (showHidden) TablerIcons.Outline.Eye else TablerIcons.Outline.EyeOff,
                        contentDescription = if (showHidden) "Hide hidden files" else "Show hidden files",
                        tint = if (showHidden) NewTheme.TextPrimary else NewTheme.TextMuted,
                        modifier = Modifier.size(20.dp),
                    )
                }
            },
        )
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                loading -> LoadingState("Browsing…", Modifier.fillMaxSize())
                error != null -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = NewTheme.Danger, modifier = Modifier.size(32.dp))
                        Text("Couldn't list that folder", color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(top = 12.dp))
                        Text(error ?: "Browse failed.", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(top = 4.dp))
                        ActionButton(text = "Retry", onClick = { browse(currentPath) }, modifier = Modifier.padding(top = 16.dp))
                    }
                }
                else -> Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 16.dp)) {
                    // "Up one level" must stay reachable when there is nothing
                    // else to tap: in a folder with no subfolders it is the only
                    // way back out, since the header back button leaves the screen.
                    if (parentPath != null) {
                        Spacer(Modifier.height(8.dp))
                        Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card)) {
                            UpOneLevelRow(onClick = { browse(parentPath) })
                        }
                    }
                    if (dirs.isEmpty()) {
                        EmptyView(title = "No subfolders", description = "This folder has no subfolders. You can still add it as a project below.", icon = TablerIcons.Outline.FolderOpen, iconTint = NewTheme.TextMuted)
                    } else {
                        Section("Folders") {
                            dirs.forEachIndexed { index, d ->
                                Row(modifier = Modifier.fillMaxWidth().clickable { browse(d.path) }.padding(horizontal = 18.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                                    Icon(TablerIcons.Outline.Folder, contentDescription = null, tint = NewTheme.TextPrimary, modifier = Modifier.size(24.dp))
                                    Text(d.name, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 16.dp))
                                    Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(20.dp))
                                }
                                if (index < dirs.lastIndex) SectionDivider(startInset = 58.dp)
                            }
                        }
                    }
                }
            }
        }
        // Footer: current folder + use-it button. No typing anywhere.
        Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(topStart = NewTheme.CardRadius, topEnd = NewTheme.CardRadius)).background(NewTheme.Card).padding(horizontal = 20.dp).padding(top = 16.dp, bottom = 24.dp)) {
            Text("Selected folder", color = NewTheme.Accent, fontSize = 13.sp, fontWeight = FontWeight.Medium)
            Text(currentPath ?: "…", color = NewTheme.TextPrimary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 4.dp, bottom = 14.dp))
            ActionButton(
                text = "Use this folder",
                onClick = {
                    val target = currentPath ?: return@ActionButton
                    adding = true
                    scope.launch {
                        try {
                            withContext(Dispatchers.IO) { AppContainer.projectRepository.addProject(target) }
                            onAdded()
                        } catch (e: Exception) {
                            confirmAlert("Failed", e.message ?: "Unable to add project.")
                        } finally { adding = false }
                    }
                },
                kind = ActionButtonKind.Primary,
                enabled = currentPath != null && !adding,
                loading = adding,
                icon = TablerIcons.Outline.Check,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}

/** Navigate to the parent directory. Rendered outside the folder list so it
 * survives in folders that have no subfolders to list. */
@Composable
private fun UpOneLevelRow(onClick: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.ChevronUp, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(22.dp))
        Text("Up one level", color = NewTheme.TextSecondary, fontSize = 16.sp, modifier = Modifier.padding(start = 16.dp))
    }
}
