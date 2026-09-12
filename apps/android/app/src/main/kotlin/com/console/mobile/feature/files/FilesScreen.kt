package com.console.mobile.feature.files

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ChevronRight
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.FolderOpen
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
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
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.model.FsTreeEntry
import com.console.mobile.data.model.getFilePreviewBlock
import com.console.mobile.data.model.isMarkdownPath
import com.console.mobile.feature.chat.MarkdownText
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Port of screens/files/files-screen.tsx + FileTreeBrowser + FileTreeRows.
 * Project-scoped lazy tree, server FFF search (350ms debounce), preview pane
 * (markdown → MarkdownText, text → mono scroll, gate → block card).
 */
@Composable
fun FilesScreen(onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val fsState by AppContainer.fsStateHolder.state.collectAsStateWithLifecycle()

    val project = projectState.projects.firstOrNull { it.id == appState.selectedProjectId } ?: projectState.projects.firstOrNull()
    val projectRoot = project?.path

    var searchQuery by remember { mutableStateOf("") }
    var debounced by remember { mutableStateOf("") }
    var searchResults by remember { mutableStateOf<List<FsTreeEntry>?>(null) }
    var searching by remember { mutableStateOf(false) }
    var entries by remember { mutableStateOf<List<FsTreeEntry>>(emptyList()) }
    var entriesLoading by remember { mutableStateOf(false) }
    var entriesError by remember { mutableStateOf<String?>(null) }
    var expanded by remember { mutableStateOf(setOf<String>()) }
    var childrenByPath by remember { mutableStateOf<Map<String, List<FsTreeEntry>>>(emptyMap()) }
    var loadingDirs by remember { mutableStateOf(setOf<String>()) }
    var selectedPath by remember { mutableStateOf<String?>(null) }
    var selectedSize by remember { mutableStateOf<Long?>(null) }
    var fileContent by remember { mutableStateOf<String?>(null) }
    var fileLoading by remember { mutableStateOf(false) }
    var fileError by remember { mutableStateOf<String?>(null) }
    var searchJob by remember { mutableStateOf<Job?>(null) }

    LaunchedEffect(projectRoot) {
        if (projectRoot == null) return@LaunchedEffect
        entriesLoading = true
        entriesError = null
        try {
            val res = withContext(Dispatchers.IO) { AppContainer.fsRepository.browseDirectory(projectRoot) }
            entries = res.entries
        } catch (e: Exception) {
            entriesError = e.message ?: "Failed to load directory."
        } finally {
            entriesLoading = false
        }
    }

    LaunchedEffect(searchQuery, projectRoot) {
        searchJob?.cancel()
        val q = searchQuery.trim()
        if (projectRoot == null || q.isEmpty()) {
            searchResults = null
            searching = false
            return@LaunchedEffect
        }
        searching = true
        val job = scope.launch {
            delay(350)
            try {
                val res = withContext(Dispatchers.IO) { AppContainer.fsRepository.searchFiles(projectRoot, q, 20, true) }
                searchResults = res.map { FsTreeEntry(name = it.absolutePath.split("/").lastOrNull() ?: it.relativePath, path = it.absolutePath, isDir = it.isDir) }
            } catch (_: Exception) {
                searchResults = emptyList()
            } finally {
                searching = false
            }
        }
        searchJob = job
    }
    @Suppress("UNUSED_EXPRESSION")
    debounced

    fun toggleDir(path: String) {
        if (expanded.contains(path)) {
            expanded = expanded - path
            return
        }
        expanded = expanded + path
        if (childrenByPath.containsKey(path) || loadingDirs.contains(path)) return
        loadingDirs = loadingDirs + path
        scope.launch {
            try {
                val kids = withContext(Dispatchers.IO) { AppContainer.consoleApi.getFsEntries(path, 1) }
                childrenByPath = childrenByPath + (path to kids)
            } catch (_: Exception) {
            } finally {
                loadingDirs = loadingDirs - path
            }
        }
    }

    fun selectFile(path: String, size: Long?) {
        selectedPath = path
        selectedSize = size
        fileContent = null
        fileError = null
        // Prefetch in background.
        scope.launch {
            fileLoading = true
            try {
                val file = withContext(Dispatchers.IO) { AppContainer.fsRepository.readFile(path) }
                if (selectedPath == path) fileContent = file.content
            } catch (e: Exception) {
                if (selectedPath == path) fileError = e.message ?: "Failed to load file."
            } finally {
                if (selectedPath == path) fileLoading = false
            }
        }
    }

    // File preview view.
    val sel = selectedPath
    if (sel != null) {
        val fileName = sel.split("/").lastOrNull() ?: sel
        val block = getFilePreviewBlock(fileName, selectedSize)
        Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
            ScreenHeader(title = fileName, subtitle = sel, onBack = { selectedPath = null })
            Box(modifier = Modifier.fillMaxSize().padding(horizontal = 12.dp).padding(bottom = 16.dp)) {
                when {
                    block != null -> Column(modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp)).padding(20.dp)) {
                        Text(block.title, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                        Text(block.message, color = ConsoleColors.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(top = 8.dp))
                    }
                    fileLoading && fileContent == null -> Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                            Text("Loading file…", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 12.dp))
                        }
                    }
                    fileError != null && fileContent == null -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            Text("Failed to load file", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                            Text(fileError ?: "", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                        }
                    }
                    else -> {
                        val content = fileContent ?: ""
                        if (isMarkdownPath(sel)) {
                            Column(modifier = Modifier.fillMaxSize().verticalScroll2().padding(4.dp)) {
                                MarkdownText(content = content)
                            }
                        } else {
                            CodePreview(content = content)
                        }
                    }
                }
            }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Files",
            subtitle = project?.name,
            onBack = onBack,
            actions = {
                IconButton(onClick = {
                    scope.launch {
                        entriesLoading = true
                        try {
                            val res = withContext(Dispatchers.IO) { AppContainer.fsRepository.browseDirectory(projectRoot) }
                            entries = res.entries
                            entriesError = null
                        } catch (e: Exception) {
                            entriesError = e.message
                        } finally {
                            entriesLoading = false
                        }
                    }
                }, modifier = Modifier.size(40.dp)) {
                    Icon(Icons.Filled.Refresh, contentDescription = "Refresh", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
                }
            },
        )
        // Search bar.
        Row(modifier = Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            OutlinedTextField(
                value = searchQuery,
                onValueChange = { searchQuery = it },
                placeholder = { Text("Search files") },
                leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(15.dp)) },
                singleLine = true,
                colors = OutlinedTextFieldDefaults.colors(focusedContainerColor = ConsoleColors.Card, unfocusedContainerColor = ConsoleColors.Card, focusedBorderColor = ConsoleColors.BorderSubtle, unfocusedBorderColor = ConsoleColors.BorderSubtle, focusedTextColor = ConsoleColors.TextPrimary, unfocusedTextColor = ConsoleColors.TextPrimary, cursorColor = ConsoleColors.TextPrimary),
                shape = RoundedCornerShape(12.dp),
                modifier = Modifier.weight(1f),
            )
            if (searchQuery.isNotEmpty()) {
                Text("Clear", color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp).clickable { searchQuery = "" })
            }
        }
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            when {
                projectRoot == null -> EmptyState(title = "No project selected", description = "Add a project folder in Settings → Projects.")
                entriesLoading && entries.isEmpty() -> Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                    CircularProgressIndicator(color = ConsoleColors.TextMuted)
                }
                entriesError != null && entries.isEmpty() -> Box(modifier = Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("Couldn't load directory", color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.Bold)
                        Text(entriesError ?: "", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
                    }
                }
                searchResults != null -> {
                    val results = searchResults ?: emptyList()
                    if (searching && results.isEmpty()) {
                        Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                            CircularProgressIndicator(color = ConsoleColors.TextMuted)
                        }
                    } else if (results.isEmpty()) {
                        EmptyState(title = "No results", description = "No files match \"$searchQuery\".")
                    } else {
                        LazyColumn(modifier = Modifier.fillMaxSize()) {
                            items(results, key = { it.path }) { r ->
                                TreeRowEntry(entry = r, depth = 0, selected = false, expanded = false, onPressDir = { toggleDir(r.path) }, onPressFile = { selectFile(r.path, null) })
                            }
                        }
                    }
                }
                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(flattenTree(entries, expanded, childrenByPath), key = { it.key }) { row ->
                        when (row.kind) {
                            TreeKind.Loading -> Row(modifier = Modifier.fillMaxWidth().padding(start = (8 + row.depth * 18).dp).padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                                CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                                Text("Loading…", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(start = 8.dp))
                            }
                            TreeKind.Empty -> Text("(empty)", color = ConsoleColors.TextMuted, fontSize = 12.sp, fontStyle = androidx.compose.ui.text.font.FontStyle.Italic, modifier = Modifier.padding(start = (8 + row.depth * 18).dp, top = 2.dp, bottom = 6.dp))
                            TreeKind.Entry -> {
                                val e = row.entry!!
                                val isLoadingDir = e.isDir && loadingDirs.contains(e.path)
                                if (isLoadingDir && !childrenByPath.containsKey(e.path)) {
                                    Row(modifier = Modifier.fillMaxWidth().padding(start = (8 + row.depth * 18).dp).padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                                        CircularProgressIndicator(color = ConsoleColors.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                                        Text(e.name, color = ConsoleColors.TextSecondary, fontSize = 14.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp))
                                    }
                                } else {
                                    TreeRowEntry(entry = e, depth = row.depth, selected = selectedPath == e.path, expanded = expanded.contains(e.path), onPressDir = { toggleDir(e.path) }, onPressFile = { selectFile(e.path, e.size) })
                                }
                            }
                        }
                    }
                }
            }
        }
        @Suppress("UNUSED_EXPRESSION")
        fsState
    }
}

private enum class TreeKind { Entry, Loading, Empty }
private data class FlatRow(val key: String, val kind: TreeKind, val entry: FsTreeEntry? = null, val depth: Int)

private fun flattenTree(roots: List<FsTreeEntry>, expanded: Set<String>, children: Map<String, List<FsTreeEntry>>): List<FlatRow> {
    val out = mutableListOf<FlatRow>()
    fun visit(list: List<FsTreeEntry>, depth: Int) {
        val sorted = list.sortedWith(compareBy({ !it.isDir }, { it.name.lowercase() }))
        for (e in sorted) {
            out.add(FlatRow("row:${e.path}", TreeKind.Entry, e, depth))
            if (e.isDir && expanded.contains(e.path)) {
                val kids = children[e.path]
                if (kids == null) {
                    out.add(FlatRow("loading:${e.path}", TreeKind.Loading, null, depth + 1))
                } else if (kids.isEmpty()) {
                    out.add(FlatRow("empty:${e.path}", TreeKind.Empty, null, depth + 1))
                } else {
                    visit(kids, depth + 1)
                }
            }
        }
    }
    visit(roots, 0)
    return out
}

@Composable
private fun TreeRowEntry(entry: FsTreeEntry, depth: Int, selected: Boolean, expanded: Boolean, onPressDir: () -> Unit, onPressFile: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp).clip(shape)
            .background(if (selected) ConsoleColors.Card else Color.Transparent)
            .border(if (selected) 1.dp else 0.dp, if (selected) ConsoleColors.Border else Color.Transparent, shape)
            .clickable { if (entry.isDir) onPressDir() else onPressFile() }
            .padding(start = (8 + depth * 18).dp, end = 8.dp, top = 10.dp, bottom = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (entry.isDir) {
            Icon(if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ChevronRight, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(12.dp))
        } else {
            Box(modifier = Modifier.size(12.dp))
        }
        if (entry.isDir) {
            Icon(
                if (expanded) Icons.Filled.FolderOpen else Icons.Filled.Folder,
                contentDescription = null,
                tint = ConsoleColors.TextSecondary,
                modifier = Modifier.size(17.dp),
            )
        } else {
            FileIcon(filename = entry.name, sizeDp = 17)
        }
        Text(entry.name, color = if (selected) ConsoleColors.TextPrimary else ConsoleColors.TextSecondary, fontSize = 14.sp, fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 8.dp))
    }
}

@Composable
private fun CodePreview(content: String) {
    // Cap render size; VirtualizedCodeView equivalent — mono + h-scroll.
    val capped = if (content.length > 200_000) content.take(200_000) + "\n…(truncated)" else content
    Box(modifier = Modifier.fillMaxSize().clip(RoundedCornerShape(12.dp)).background(Color(0xFF101113)).border(1.dp, Color.White.copy(alpha = 0.08f), RoundedCornerShape(12.dp)).padding(12.dp)) {
        Text(
            capped,
            color = Color(0xFFE4E4E7),
            fontSize = 11.sp,
            fontFamily = ConsoleMonoFamily,
            lineHeight = 17.sp,
            modifier = Modifier.fillMaxSize().hScroll2(),
        )
    }
}

@Composable
private fun Modifier.verticalScroll2(): Modifier = this.verticalScroll(androidx.compose.foundation.rememberScrollState())
@Composable
private fun Modifier.hScroll2(): Modifier = this.horizontalScroll(androidx.compose.foundation.rememberScrollState())
