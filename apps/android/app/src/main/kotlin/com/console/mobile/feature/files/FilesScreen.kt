package com.console.mobile.feature.files

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import console.v1.FsTreeEntry
import com.console.mobile.data.model.getFilePreviewBlock
import com.console.mobile.data.model.isMarkdownPath
import com.console.mobile.feature.chat.markdown.CustomMarkdown
import com.console.mobile.ui.code.CodeViewer
import com.console.mobile.ui.code.languageForPath
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.CircleIconButton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.SearchInput
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Search

/**
 * Port of screens/files/files-screen.tsx + FileTreeBrowser + FileTreeRows.
 * Project-scoped lazy tree, server FFF search (350ms debounce), preview pane
 * (markdown → CustomMarkdown, text → mono scroll, gate → block card).
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
    val resultsListState = rememberLazyListState()
    var searching by remember { mutableStateOf(false) }
    var entries by remember { mutableStateOf<List<FsTreeEntry>>(emptyList()) }
    var entriesLoading by remember { mutableStateOf(false) }
    var entriesError by remember { mutableStateOf<String?>(null) }
    var expanded by remember { mutableStateOf(setOf<String>()) }
    var childrenByPath by remember { mutableStateOf<Map<String, List<FsTreeEntry>>>(emptyMap()) }
    var loadingDirs by remember { mutableStateOf(setOf<String>()) }
    var selectedPath by remember { mutableStateOf<String?>(null) }
    BackHandler(enabled = selectedPath != null) { selectedPath = null }
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
                searchResults = res.map { FsTreeEntry(name = it.absolute_path.split("/").lastOrNull() ?: it.relative_path, path = it.absolute_path, is_dir = it.is_dir) }
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
        val relativePath = projectRoot?.let { root ->
            val normalizedRoot = root.trimEnd('/')
            if (sel.startsWith("$normalizedRoot/")) sel.removePrefix("$normalizedRoot/") else sel
        } ?: sel
        val subtitle = listOfNotNull(project?.name?.takeIf { it.isNotBlank() }, relativePath).joinToString(" · ")
        Column(modifier = Modifier.fillMaxSize().background(NewTheme.Black)) {
            PageHeader(title = fileName, subtitle = subtitle, onBack = { selectedPath = null })
            Box(modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp).padding(bottom = 16.dp)) {
                when {
                    block != null -> Column(modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card).padding(20.dp)) {
                        Text(block.title, color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
                        Text(block.message, color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(top = 8.dp))
                    }
                    fileLoading && fileContent == null -> LoadingState("Loading file…", Modifier.fillMaxSize())
                    fileError != null && fileContent == null -> EmptyView(title = "Failed to load file", description = fileError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                    else -> {
                        val content = fileContent ?: ""
                        if (isMarkdownPath(sel)) {
                            Column(modifier = Modifier.fillMaxSize().verticalScroll2().padding(4.dp)) {
                                CustomMarkdown(content = content)
                            }
                        } else {
                            CodePreview(content = content, path = sel)
                        }
                    }
                }
            }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Black)) {
        PageHeader(
            title = "Files",
            subtitle = project?.name,
            onBack = onBack,
            actions = {
                CircleIconButton(TablerIcons.Outline.Refresh, "Refresh", onClick = {
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
                })
            },
        )
        // Search bar.
        SearchInput(
            value = searchQuery,
            onValueChange = { searchQuery = it },
            placeholder = "Search files",
            modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 12.dp),
        )
        Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
            // Scrolling the results means the user has seen enough to act on
            // one of them, so get the keyboard out of the way.
            val keyboard = LocalSoftwareKeyboardController.current
            LaunchedEffect(resultsListState) {
                snapshotFlow { resultsListState.isScrollInProgress }
                    .collect { scrolling -> if (scrolling) keyboard?.hide() }
            }
            when {
                projectRoot == null -> EmptyView(title = "No project selected", description = "Add a project folder in Settings → Projects.", icon = TablerIcons.Outline.FolderOpen, modifier = Modifier.fillMaxSize())
                entriesLoading && entries.isEmpty() -> LoadingState(modifier = Modifier.fillMaxSize())
                entriesError != null && entries.isEmpty() -> EmptyView(title = "Couldn't load directory", description = entriesError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                searchResults != null -> {
                    val results = searchResults ?: emptyList()
                    if (searching && results.isEmpty()) {
                        LoadingState(modifier = Modifier.fillMaxSize())
                    } else if (results.isEmpty()) {
                        EmptyView(title = "No results", description = "No files match \"$searchQuery\".", icon = TablerIcons.Outline.Search, modifier = Modifier.fillMaxSize())
                    } else {
                        LazyColumn(state = resultsListState, modifier = Modifier.fillMaxSize()) {
                            items(results, key = { it.path }) { r ->
                                TreeRowEntry(entry = r, depth = 0, selected = false, expanded = false, onPressDir = { toggleDir(r.path) }, onPressFile = { selectFile(r.path, null) })
                            }
                        }
                    }
                }
                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(flattenTree(entries, expanded, childrenByPath), key = { it.key }) { row ->
                        when (row.kind) {
                            TreeKind.Loading -> Row(modifier = Modifier.fillMaxWidth().padding(start = (16 + row.depth * 18).dp).padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                                CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                                Text("Loading…", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(start = 8.dp))
                            }
                            TreeKind.Empty -> Text("(empty)", color = NewTheme.TextMuted, fontSize = 13.sp, fontStyle = androidx.compose.ui.text.font.FontStyle.Italic, modifier = Modifier.padding(start = (16 + row.depth * 18).dp, top = 2.dp, bottom = 8.dp))
                            TreeKind.Entry -> {
                                val e = row.entry!!
                                val isLoadingDir = e.is_dir && loadingDirs.contains(e.path)
                                if (isLoadingDir && !childrenByPath.containsKey(e.path)) {
                                    Row(modifier = Modifier.fillMaxWidth().padding(start = (16 + row.depth * 18).dp).padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                                        CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                                        Text(e.name, color = NewTheme.TextSecondary, fontSize = 15.sp, modifier = Modifier.padding(start = 8.dp))
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
        val sorted = list.sortedWith(compareBy({ !it.is_dir }, { it.name.lowercase() }))
        for (e in sorted) {
            out.add(FlatRow("row:${e.path}", TreeKind.Entry, e, depth))
            if (e.is_dir && expanded.contains(e.path)) {
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
    Row(
        modifier = Modifier.fillMaxWidth()
            .background(if (selected) Color.White.copy(alpha = 0.06f) else Color.Transparent)
            .clickable { if (entry.is_dir) onPressDir() else onPressFile() }
            .padding(start = (16 + depth * 18).dp, end = 16.dp, top = 12.dp, bottom = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (entry.is_dir) {
            Icon(if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(14.dp))
        } else {
            Box(modifier = Modifier.size(14.dp))
        }
        Box(modifier = Modifier.padding(start = 8.dp)) {
            if (entry.is_dir) {
                Icon(
                    if (expanded) TablerIcons.Outline.FolderOpen else TablerIcons.Outline.Folder,
                    contentDescription = null,
                    tint = NewTheme.TextPrimary,
                    modifier = Modifier.size(20.dp),
                )
            } else {
                FileIcon(filename = entry.name, sizeDp = 20)
            }
        }
        Text(entry.name, color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f).padding(start = 12.dp))
    }
}

@Composable
private fun CodePreview(content: String, path: String?) {
    // Cap render size; Sora virtualizes so 2k+ line files stay smooth.
    val capped = if (content.length > 200_000) content.take(200_000) + "\n…(truncated)" else content
    val language = remember(path) { languageForPath(path) }
    CodeViewer(
        code = capped,
        language = language,
        modifier = Modifier.fillMaxSize().padding(horizontal = 4.dp, vertical = 8.dp),
        showLineNumbers = true,
        // The screen is pure black, so the pinned gutter has to be too.
        gutterColor = NewTheme.Black,
    )
}

@Composable
private fun Modifier.verticalScroll2(): Modifier = this.verticalScroll(androidx.compose.foundation.rememberScrollState())
@Composable
private fun Modifier.hScroll2(): Modifier = this.horizontalScroll(androidx.compose.foundation.rememberScrollState())
