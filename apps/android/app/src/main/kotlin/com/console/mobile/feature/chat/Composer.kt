package com.console.mobile.feature.chat

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.exclude
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.SmartToy
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.rememberModalBottomSheetState
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
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.core.util.formatModelName
import com.console.mobile.data.model.ApprovalMode
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.Model
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.components.attachmentBytes
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Port of components/chat/composer (composer, composer-input, composer-bottom-strip,
 * attachment-strip, selectors). Multiline input + send/stop + image attach +
 * project/model/approval pickers.
 */
@Composable
fun Composer(
    sessionId: String,
    value: String,
    onChange: (String) -> Unit,
    running: Boolean,
    projectLocked: Boolean,
    topBanner: (@Composable () -> Unit)? = null,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val attachments = chatSessions[sessionId]?.attachments ?: emptyList()
    val canSend = value.trim().isNotEmpty() || attachments.isNotEmpty()

    val pickImages = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris: List<Uri> ->
        if (uris.isEmpty()) return@rememberLauncherForActivityResult
        scope.launch {
            val list = withContext(Dispatchers.IO) {
                uris.take(4).mapNotNull { uri ->
                    try {
                        val mime = context.contentResolver.getType(uri) ?: "image/jpeg"
                        context.contentResolver.openInputStream(uri)?.use { ins ->
                            val bytes = ins.readBytes()
                            if (bytes.size > 10 * 1024 * 1024) return@mapNotNull null
                            ImageAttachment(data = android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP), mimeType = mime)
                        }
                    } catch (_: Exception) { null }
                }
            }
            if (list.isNotEmpty()) AppContainer.chatRepository.addAttachments(sessionId, list)
        }
    }

    // ime minus nav bars: Scaffold already pads the nav bar, so only lift
    // by the keyboard itself — otherwise the gap doubles when typing.
    Column(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Background).windowInsetsPadding(WindowInsets.ime.exclude(WindowInsets.navigationBars)).padding(horizontal = 10.dp).padding(top = 8.dp, bottom = 8.dp)) {
        if (topBanner != null) topBanner()
        if (attachments.isNotEmpty()) {
            AttachmentStrip(sessionId = sessionId, attachments = attachments)
        }
        Row(
            modifier = Modifier.fillMaxWidth().clip(if (value.contains("\n")) RoundedCornerShape(20.dp) else CircleShape)
                .background(ConsoleColors.Card)
                .border(1.dp, ConsoleColors.Border, if (value.contains("\n")) RoundedCornerShape(20.dp) else CircleShape)
                .padding(horizontal = 6.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                modifier = Modifier.size(28.dp).clip(CircleShape)
                    .clickable(onClickLabel = "Attach image") { pickImages.launch("image/*") },
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.Filled.Add, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(18.dp))
            }
            BasicTextField(
                value = value,
                onValueChange = onChange,
                modifier = Modifier
                    .weight(1f)
                    .padding(horizontal = 8.dp)
                    .heightIn(max = 120.dp),
                textStyle = androidx.compose.ui.text.TextStyle(
                    color = ConsoleColors.TextPrimary,
                    fontSize = 14.sp,
                    lineHeight = 19.sp,
                ),
                cursorBrush = SolidColor(ConsoleColors.TextPrimary),
                maxLines = 6,
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty()) {
                            Text("Ask anything…", color = ConsoleColors.TextMuted, fontSize = 14.sp)
                        }
                        innerTextField()
                    }
                },
            )
            if (running) {
                Box(
                    modifier = Modifier.size(28.dp).clip(CircleShape).background(Color.White)
                        .clickable(onClickLabel = "Stop") { onStop() },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Filled.Stop, contentDescription = null, tint = Color.Black, modifier = Modifier.size(11.dp))
                }
            } else {
                Box(
                    modifier = Modifier.size(28.dp).clip(CircleShape)
                        .background(if (canSend) Color.White else Color.White.copy(alpha = 0.08f))
                        .clickable(enabled = canSend, onClickLabel = "Send") { onSend() },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Filled.ArrowUpward, contentDescription = null, tint = if (canSend) Color.Black else ConsoleColors.TextMuted, modifier = Modifier.size(14.dp))
                }
            }
        }
        ComposerBottomStrip(sessionId = sessionId, projectLocked = projectLocked)
    }
}

@Composable
private fun AttachmentStrip(sessionId: String, attachments: List<ImageAttachment>) {
    var preview by remember { mutableStateOf<ImageAttachment?>(null) }
    Row(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(bottom = 8.dp, start = 4.dp)) {
        attachments.forEachIndexed { idx, att ->
            val bytes = remember(att) { attachmentBytes(att.data) }
            Box(modifier = Modifier.padding(end = 8.dp).size(56.dp).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp)).clickable { preview = att }) {
                if (bytes != null) {
                    coil3.compose.AsyncImage(
                        model = bytes,
                        contentDescription = "Attachment ${idx + 1}",
                        modifier = Modifier.size(56.dp).clip(RoundedCornerShape(12.dp)),
                        contentScale = androidx.compose.ui.layout.ContentScale.Crop,
                    )
                } else {
                    Box(modifier = Modifier.size(56.dp), contentAlignment = Alignment.Center) {
                        Icon(Icons.Filled.Image, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(20.dp))
                    }
                }
                Box(modifier = Modifier.align(Alignment.TopEnd).padding(2.dp).size(18.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.6f)).clickable { AppContainer.chatRepository.removeAttachment(sessionId, idx) }, contentAlignment = Alignment.Center) {
                    Icon(Icons.Filled.Close, contentDescription = "Remove", tint = Color.White, modifier = Modifier.size(12.dp))
                }
            }
        }
    }
    val current = preview
    if (current != null && attachments.contains(current)) {
        val bytes = remember(current) { attachmentBytes(current.data) }
        if (bytes != null) {
            ImagePreviewDialog(image = bytes, onDismiss = { preview = null })
        }
    }
}

@Composable
private fun ComposerBottomStrip(sessionId: String, projectLocked: Boolean) {
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val sessionViews by AppContainer.sessionStateHolder.views.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val view = sessionViews[sessionId]
    val projects = projectState.projects
    var projectSheet by remember { mutableStateOf(false) }
    var modelSheet by remember { mutableStateOf(false) }
    var approvalSheet by remember { mutableStateOf(false) }

    val selectedProject = projects.firstOrNull { p ->
        (view?.sessionCwd?.isNotEmpty() == true && (p.path == view.sessionCwd || view.sessionCwd.startsWith(p.path + "/"))) || p.id == null
    } ?: projects.firstOrNull { it.path == view?.sessionCwd }
    val modelLabel = view?.sessionModelId?.ifBlank { null }?.let { formatModelName(it) } ?: "Default Model"
    val modeLabel = ApprovalMode.fromValue(view?.approvalMode ?: "").let {
        when (it) {
            ApprovalMode.AlwaysAsk -> "Always Ask"
            ApprovalMode.AcceptEdits -> "Accept Edits"
            ApprovalMode.PlanMode -> "Plan Mode"
            ApprovalMode.FullAccess -> "Full Access"
        }
    }

    LazyRow(modifier = Modifier.fillMaxWidth().padding(top = 8.dp, start = 6.dp, end = 6.dp, bottom = 4.dp)) {
        item {
            PickerChip(icon = if (projectLocked) Icons.Filled.Lock else Icons.Filled.Folder, label = selectedProject?.name ?: "Select Folder", modifier = Modifier.padding(end = 8.dp)) {
                if (!projectLocked) {
                    if (projects.isEmpty()) AppContainer.projectRepository.loadProjects()
                    projectSheet = true
                }
            }
        }
        item {
            PickerChip(icon = Icons.Filled.SmartToy, label = modelLabel, modifier = Modifier.padding(end = 8.dp)) {
                AppContainer.providerRepository.loadProviders()
                modelSheet = true
            }
        }
        item {
            PickerChip(icon = Icons.Filled.Shield, label = modeLabel) {
                approvalSheet = true
            }
        }
    }

    if (projectSheet) {
        ProjectPickerSheet(projects = projects, selectedId = selectedProject?.id, locked = projectLocked, onDismiss = { projectSheet = false }, onSelect = { proj ->
            projectSheet = false
            scope.launch {
                val dto = UpdateSessionDto(cwd = proj.path)
                AppContainer.projectRepository.updateSession(sessionId, dto)
                AppContainer.sessionRepository.refreshHeader(sessionId)
            }
        })
    }
    if (modelSheet) {
        ModelPickerSheet(selectedModel = view?.sessionModelId, selectedProvider = view?.sessionProvider, onDismiss = { modelSheet = false }, onSelect = { modelId, provider ->
            modelSheet = false
            scope.launch {
                val dto = UpdateSessionDto(modelId = modelId, provider = provider)
                AppContainer.projectRepository.updateSession(sessionId, dto)
                AppContainer.sessionRepository.refreshHeader(sessionId)
            }
        })
    }
    if (approvalSheet) {
        ApprovalModePickerSheet(current = view?.approvalMode ?: ApprovalMode.AlwaysAsk.value, onDismiss = { approvalSheet = false }, onSelect = { mode ->
            approvalSheet = false
            scope.launch {
                val dto = UpdateSessionDto(approvalMode = mode)
                AppContainer.projectRepository.updateSession(sessionId, dto)
                AppContainer.sessionRepository.refreshHeader(sessionId)
            }
        })
    }
}

@Composable
private fun PickerChip(icon: ImageVector, label: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    Row(
        modifier = modifier.clip(RoundedCornerShape(8.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp)).clickable { onClick() }.padding(horizontal = 9.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(13.dp))
        Text(label, color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontWeight = FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(start = 5.dp))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ProjectPickerSheet(projects: List<ProjectInfo>, selectedId: String?, locked: Boolean, onDismiss: () -> Unit, onSelect: (ProjectInfo) -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Text("Select Folder", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
            if (locked) {
                Text("Folder cannot be changed once a chat has started.", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(bottom = 12.dp))
            } else {
                projects.forEach { p ->
                    val sel = p.id == selectedId
                    Row(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp).clip(RoundedCornerShape(12.dp)).background(if (sel) ConsoleColors.CardAlt else Color.Transparent).border(1.dp, if (sel) ConsoleColors.Border else Color.Transparent, RoundedCornerShape(12.dp)).clickable { onSelect(p) }.padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(p.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                            Text(p.path, color = ConsoleColors.TextSecondary, fontSize = 11.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, fontFamily = ConsoleMonoFamily)
                        }
                        if (sel) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(16.dp))
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ModelPickerSheet(selectedModel: String?, selectedProvider: String?, onDismiss: () -> Unit, onSelect: (String, String?) -> Unit) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    var search by remember { mutableStateOf("") }
    var activeProvider by remember(providerState.providers) {
        mutableStateOf(selectedProvider ?: providerState.providers.firstOrNull()?.name)
    }
    LaunchedEffect(Unit) { AppContainer.providerRepository.loadProviders() }
    LaunchedEffect(activeProvider) { activeProvider?.let { AppContainer.providerRepository.loadModels(it) } }

    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Text("Select Model", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
            if (providerState.providers.isNotEmpty()) {
                LazyRow(modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp)) {
                    items(providerState.providers) { p ->
                        val sel = p.name == activeProvider
                        Box(modifier = Modifier.padding(end = 8.dp).clip(RoundedCornerShape(8.dp)).background(if (sel) Color.White.copy(alpha = 0.12f) else ConsoleColors.CardAlt).clickable { activeProvider = p.name }.padding(horizontal = 10.dp, vertical = 6.dp)) {
                            Text(p.displayName.ifBlank { p.name }, color = if (sel) ConsoleColors.TextPrimary else ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium)
                        }
                    }
                }
            }
            OutlinedTextField(value = search, onValueChange = { search = it }, placeholder = { Text("Search models…") }, leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(14.dp)) }, singleLine = true, colors = OutlinedTextFieldDefaults.colors(focusedContainerColor = ConsoleColors.CardAlt, unfocusedContainerColor = ConsoleColors.CardAlt, focusedBorderColor = ConsoleColors.BorderSubtle, unfocusedBorderColor = ConsoleColors.BorderSubtle, focusedTextColor = ConsoleColors.TextPrimary, unfocusedTextColor = ConsoleColors.TextPrimary, cursorColor = ConsoleColors.TextPrimary), shape = RoundedCornerShape(12.dp), modifier = Modifier.fillMaxWidth().padding(bottom = 12.dp))
            val models: List<Model> = activeProvider?.let { providerState.modelsByProvider[it] } ?: emptyList()
            val q = search.trim().lowercase()
            val filtered = if (q.isEmpty()) models else models.filter { it.id.lowercase().contains(q) }
            val loading = activeProvider?.let { providerState.loadingModels[it] } == true || providerState.loadingProviders
            if (loading && models.isEmpty()) {
                Box(modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                }
            } else if (filtered.isEmpty()) {
                Text(if (search.isNotEmpty()) "No matching models found" else "No models available", color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(vertical = 32.dp))
            } else {
                Column {
                    filtered.take(100).forEach { m ->
                        val sel = m.id == selectedModel
                        Row(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp).clip(RoundedCornerShape(12.dp)).background(if (sel) ConsoleColors.CardAlt else Color.Transparent).border(1.dp, if (sel) ConsoleColors.Border else Color.Transparent, RoundedCornerShape(12.dp)).clickable { onSelect(m.id, activeProvider) }.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text(formatModelName(m.id), color = ConsoleColors.TextPrimary, fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                                Text(m.id, color = ConsoleColors.TextSecondary, fontSize = 10.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            }
                            if (sel) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(14.dp))
                        }
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ApprovalModePickerSheet(current: String, onDismiss: () -> Unit, onSelect: (String) -> Unit) {
    val providerState by AppContainer.providerStateHolder.state.collectAsStateWithLifecycle()
    LaunchedEffect(Unit) { AppContainer.providerRepository.loadApprovalModes() }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true), containerColor = ConsoleColors.Background) {
        Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 20.dp).padding(bottom = 40.dp)) {
            Text("Approval Mode", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(bottom = 12.dp))
            if (providerState.loadingApprovalModes && providerState.approvalModes.isEmpty()) {
                Box(modifier = Modifier.fillMaxWidth().padding(vertical = 32.dp), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                }
            } else {
                val modes = providerState.approvalModes.ifEmpty {
                    ApprovalMode.entries.map { com.console.mobile.data.model.ApprovalModeOption(it, it.value, "") }
                }
                modes.forEach { m ->
                    val sel = m.value.value == current
                    Row(modifier = Modifier.fillMaxWidth().padding(bottom = 6.dp).clip(RoundedCornerShape(12.dp)).background(if (sel) ConsoleColors.CardAlt else Color.Transparent).border(1.dp, if (sel) ConsoleColors.Border else Color.Transparent, RoundedCornerShape(12.dp)).clickable { onSelect(m.value.value) }.padding(horizontal = 14.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(modifier = Modifier.weight(1f)) {
                            Text(m.label.ifBlank { m.value.value }, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                            if (m.description.isNotBlank()) Text(m.description, color = ConsoleColors.TextSecondary, fontSize = 12.sp, modifier = Modifier.padding(top = 2.dp))
                        }
                        if (sel) Icon(Icons.Filled.Check, contentDescription = null, tint = Color(0xFF34D399), modifier = Modifier.size(16.dp))
                    }
                }
            }
        }
    }
}
