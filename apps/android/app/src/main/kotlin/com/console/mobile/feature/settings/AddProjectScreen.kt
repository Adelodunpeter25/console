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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
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
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.PillButton
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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
    var dirs by remember { mutableStateOf<List<com.console.mobile.data.model.FsTreeEntry>>(emptyList()) }
    var loading by remember { mutableStateOf(true) }
    var error by remember { mutableStateOf<String?>(null) }
    var adding by remember { mutableStateOf(false) }

    fun browse(path: String?) {
        loading = true
        error = null
        scope.launch {
            try {
                val res = withContext(Dispatchers.IO) { AppContainer.fsRepository.browseDirectory(path) }
                currentPath = res.currentPath
                parentPath = res.parentPath
                dirs = res.entries.filter { it.isDir }
            } catch (e: Exception) {
                error = e.message ?: "Couldn't list that folder."
            } finally {
                loading = false
            }
        }
    }

    LaunchedEffect(Unit) { browse(null) }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(title = "Add Project", subtitle = currentPath, onBack = onBack)
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                loading -> Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                        Text("Browsing…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                    }
                }
                error != null -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Icon(Icons.Filled.Warning, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(32.dp))
                        Text("Couldn't list that folder", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 12.dp))
                        Text(error ?: "Browse failed.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                        PillButton(text = "Retry", onClick = { browse(currentPath) }, modifier = Modifier.padding(top = 16.dp))
                    }
                }
                dirs.isEmpty() -> EmptyState(title = "No subfolders", description = "This folder has no subfolders. You can still add it as a project below.", icon = { Icon(Icons.Filled.Folder, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(32.dp)) })
                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    if (parentPath != null) {
                        item(key = "up") {
                            Row(modifier = Modifier.fillMaxWidth().clickable { browse(parentPath) }.padding(horizontal = 20.dp, vertical = 14.dp), verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Filled.ArrowUpward, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp).padding(end = 4.dp))
                                Text("Up one level", color = ConsoleColors.TextSecondary, fontSize = 13.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp))
                            }
                        }
                    }
                    items(dirs, key = { it.path }) { d ->
                        Row(modifier = Modifier.fillMaxWidth().clickable { browse(d.path) }.padding(horizontal = 20.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                            Box(modifier = Modifier.size(32.dp).clip(RoundedCornerShape(8.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp)), contentAlignment = Alignment.Center) {
                                Icon(Icons.Filled.Folder, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
                            }
                            Text(d.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 12.dp))
                        }
                    }
                }
            }
        }
        // Footer: current folder + use-it button. No typing anywhere.
        Column(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Card).padding(horizontal = 20.dp).padding(top = 12.dp, bottom = 24.dp)) {
            Text("Selected folder", color = ConsoleColors.TextMuted, fontSize = 11.sp, fontWeight = FontWeight.SemiBold)
            Text(currentPath ?: "…", color = ConsoleColors.TextPrimary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp, bottom = 12.dp))
            PillButton(
                text = "Use this folder",
                onClick = {
                    val target = currentPath ?: return@PillButton
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
                enabled = currentPath != null && !adding,
                loading = adding,
                fullWidth = true,
                icon = Icons.Filled.Check,
            )
        }
    }
}
