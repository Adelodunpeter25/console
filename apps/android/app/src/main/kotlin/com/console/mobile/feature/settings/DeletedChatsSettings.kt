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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Message
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Restore
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
import com.console.mobile.AppContainer
import com.console.mobile.core.util.folderName
import com.console.mobile.core.util.formatRelativeTime
import com.console.mobile.data.model.SessionHeader
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Port of screens/settings/deleted-chats-settings.tsx. */
@Composable
fun DeletedChatsSettings(onBack: () -> Unit) {
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    var busyId by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()
    val deleted = projectState.deletedSessions

    LaunchedEffect(Unit) { AppContainer.projectRepository.loadDeletedSessions() }

    fun restore(id: String) {
        busyId = id
        scope.launch {
            try {
                withContext(Dispatchers.IO) { AppContainer.projectRepository.restoreSession(id) }
            } catch (e: Exception) {
                confirmAlert("Failed", e.message ?: "Unable to restore chat.")
            } finally { busyId = null }
        }
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Deleted Chats",
            onBack = onBack,
            actions = if (deleted.isNotEmpty()) {
                {
                    IconButton(onClick = {
                        confirmAlert("Restore All Chats", "Restore all ${deleted.size} deleted chats back to your workspace?", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Restore All", onPress = {
                            busyId = "all"
                            scope.launch {
                                try {
                                    withContext(Dispatchers.IO) {
                                        for (s in AppContainer.projectStateHolder.state.value.deletedSessions) AppContainer.projectRepository.restoreSession(s.id)
                                    }
                                } catch (e: Exception) {
                                    confirmAlert("Failed", e.message ?: "Unable to restore all chats.")
                                } finally { busyId = null }
                            }
                        })))
                    }, modifier = Modifier.size(36.dp)) {
                        Icon(Icons.Filled.Restore, contentDescription = "Restore all", tint = ConsoleColors.TextPrimary, modifier = Modifier.size(17.dp))
                    }
                    IconButton(onClick = {
                        confirmAlert("Delete All Chats Permanently", "All ${deleted.size} deleted chats and their entire message history will be permanently removed. This cannot be undone.", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Delete All", destructive = true, onPress = {
                            busyId = "all"
                            scope.launch {
                                try {
                                    withContext(Dispatchers.IO) {
                                        for (s in AppContainer.projectStateHolder.state.value.deletedSessions) AppContainer.projectRepository.permanentlyDeleteSession(s.id)
                                    }
                                } catch (e: Exception) {
                                    confirmAlert("Failed", e.message ?: "Unable to delete all chats.")
                                } finally { busyId = null }
                            }
                        })))
                    }, enabled = busyId == null, modifier = Modifier.size(36.dp)) {
                        Icon(Icons.Filled.Delete, contentDescription = "Delete all", tint = ConsoleColors.Destructive, modifier = Modifier.size(17.dp))
                    }
                }
            } else null,
        )
        if (projectState.deletedLoading && deleted.isEmpty()) {
            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                    Text("Loading deleted chats…", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                }
            }
        } else if (deleted.isEmpty()) {
            EmptyState(title = "No deleted chats", description = "Chats you delete will appear here until permanently purged.", icon = { Icon(Icons.Filled.Message, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(32.dp)) })
        } else {
            LazyColumn(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
                items(deleted, key = { it.id }) { item ->
                    DeletedRow(item = item, busy = busyId == item.id || busyId == "all", onRestore = { restore(item.id) }, onDelete = {
                        val title = item.title.ifBlank { "Untitled Chat" }
                        confirmAlert("Delete Chat Permanently", "\"$title\" and its message history will be permanently deleted. This cannot be undone.", listOf(ConfirmButton("Cancel", cancel = true), ConfirmButton("Delete", destructive = true, onPress = {
                            busyId = item.id
                            scope.launch {
                                try {
                                    withContext(Dispatchers.IO) { AppContainer.projectRepository.permanentlyDeleteSession(item.id) }
                                } catch (e: Exception) {
                                    confirmAlert("Failed", e.message ?: "Unable to delete chat.")
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
private fun DeletedRow(item: SessionHeader, busy: Boolean, onRestore: () -> Unit, onDelete: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(shape).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, shape).padding(14.dp)) {
        Text(item.title.ifBlank { "Untitled Chat" }, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
        val ts = item.deletedAt ?: item.updatedAt
        Text("${folderName(item.cwd)} · Deleted ${formatRelativeTime(ts).ifBlank { "recently" }}", color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 2.dp))
        Row(modifier = Modifier.fillMaxWidth().padding(top = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Box(modifier = Modifier.weight(1f))
            TextButton(onClick = onRestore, enabled = !busy, modifier = Modifier.clip(RoundedCornerShape(8.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp)).padding(horizontal = 12.dp, vertical = 6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (busy) CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(12.dp))
                    else Icon(Icons.Filled.Refresh, contentDescription = null, tint = Color.White, modifier = Modifier.size(12.dp))
                    Text("Restore", color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
                }
            }
            TextButton(onClick = onDelete, enabled = !busy, modifier = Modifier.padding(start = 8.dp).clip(RoundedCornerShape(8.dp)).background(ConsoleColors.Destructive.copy(alpha = 0.1f)).border(1.dp, ConsoleColors.Destructive.copy(alpha = 0.3f), RoundedCornerShape(8.dp)).padding(horizontal = 12.dp, vertical = 6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Filled.Delete, contentDescription = null, tint = ConsoleColors.Destructive, modifier = Modifier.size(12.dp))
                    Text("Delete", color = ConsoleColors.Destructive, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 6.dp))
                }
            }
        }
    }
}
