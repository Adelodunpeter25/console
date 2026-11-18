package com.console.mobile.feature.files

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.FileProvider
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Code
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import com.console.mobile.AppContainer
import console.v1.FsTreeEntry
import com.console.mobile.SharedNative.GitBadgeInfo
import com.console.mobile.core.util.gitBadges
import com.console.mobile.core.util.gitRoleColor
import com.console.mobile.data.model.FileDetails
import com.console.mobile.data.model.FilePreviewKind
import com.console.mobile.data.model.fileDetails
import com.console.mobile.data.model.filePreviewKind
import com.console.mobile.data.model.getFilePreviewBlock
import com.console.mobile.data.model.mimeTypeForName
import com.console.mobile.core.util.closestMatch
import com.console.mobile.core.util.isBareFileName
import com.console.mobile.core.util.parseLineSuffix
import com.console.mobile.core.util.resolveFilePath
import com.console.mobile.core.util.stripLineSuffix
import com.console.mobile.data.model.isHtmlFileName
import com.console.mobile.data.model.isMarkdownPath
import com.console.mobile.feature.browser.handOffToSystem
import com.console.mobile.feature.chat.html.HtmlPreviewWebView
import com.console.mobile.feature.chat.markdown.CustomMarkdown
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.code.CodeViewer
import com.console.mobile.ui.code.languageForPath
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.theme.ConsoleMonoFamily
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import coil3.svg.SvgDecoder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.PageHeaderHeight
import com.console.mobile.ui.components.common.new.SearchInput
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.ExternalLink
import io.github.lyxnx.compose.ui.tablericons.outline.FileTypePdf
import io.github.lyxnx.compose.ui.tablericons.outline.Music
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerPlay
import io.github.lyxnx.compose.ui.tablericons.outline.Search
import io.github.lyxnx.compose.ui.tablericons.outline.Video
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.pdf.PdfRenderer
import androidx.compose.foundation.Image
import android.net.Uri
import android.os.ParcelFileDescriptor
import androidx.media3.common.MediaItem
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.ui.PlayerView
import java.io.File
import java.io.FileOutputStream

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
    // The git badge of every changed path, and of every folder above one.
    var gitBadges by remember { mutableStateOf<Map<String, GitBadgeInfo>>(emptyMap()) }
    var selectedPath by remember { mutableStateOf<String?>(null) }
    // True while the open file came from a chat link rather than the tree.
    var openedFromLink by remember { mutableStateOf(false) }
    var targetLine by remember { mutableStateOf<Int?>(null) }
    // System back follows where you came from; the header arrow always shows the tree.
    BackHandler(enabled = selectedPath != null) {
        when (previewBackAction(openedFromLink)) {
            PreviewBack.ToPreviousScreen -> { openedFromLink = false; onBack() }
            PreviewBack.ToFileTree -> selectedPath = null
        }
    }
    var selectedSize by remember { mutableStateOf<Long?>(null) }
    var fileContent by remember { mutableStateOf<String?>(null) }
    // Raw bytes for IMAGE and PDF previews; the URL (token included) for VIDEO/AUDIO.
    var fileBytes by remember { mutableStateOf<ByteArray?>(null) }
    var mediaUrl by remember { mutableStateOf<String?>(null) }
    var htmlAsEditor by remember { mutableStateOf(false) }
    var pendingHtmlChoice by remember { mutableStateOf<Pair<String, Boolean>?>(null) }
    var fileLoading by remember { mutableStateOf(false) }
    var fileError by remember { mutableStateOf<String?>(null) }
    // Tapping a previewed image opens the fullscreen zoom dialog.
    var zoomImage by remember { mutableStateOf<ByteArray?>(null) }
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
        gitBadges = loadGitBadges(projectRoot)
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

    fun loadTextContent(path: String) {
        scope.launch {
            fileLoading = true
            fileError = null
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

    fun selectFile(path: String, size: Long?, fromLink: Boolean = false, line: Int? = null, forceEditor: Boolean = false) {
        openedFromLink = fromLink
        targetLine = line
        selectedPath = path
        selectedSize = size
        fileContent = null
        fileBytes = null
        mediaUrl = null
        fileError = null
        htmlAsEditor = forceEditor
        val kind = filePreviewKind(path.split("/").lastOrNull() ?: path)
        if (kind == FilePreviewKind.HTML && !forceEditor) {
            fileLoading = false
            return
        }
        scope.launch {
            fileLoading = true
            try {
                when (kind) {
                    FilePreviewKind.IMAGE, FilePreviewKind.PDF -> {
                        val bytes = withContext(Dispatchers.IO) { AppContainer.fsRepository.readFileRaw(path) }
                        if (selectedPath == path) fileBytes = bytes
                    }
                    FilePreviewKind.VIDEO, FilePreviewKind.AUDIO -> {
                        val url = withContext(Dispatchers.IO) { AppContainer.consoleApi.fsMediaUrl(path) }
                        if (selectedPath == path) mediaUrl = url
                    }
                    else -> {
                        val file = withContext(Dispatchers.IO) { AppContainer.fsRepository.readFile(path) }
                        if (selectedPath == path) fileContent = file.content
                    }
                }
            } catch (e: Exception) {
                if (selectedPath == path) fileError = e.message ?: "Failed to load file."
            } finally {
                if (selectedPath == path) fileLoading = false
            }
        }
    }

    // A link in chat asked for this file: open it once the project is known.
    val pendingFile = appState.pendingFilePath
    LaunchedEffect(pendingFile, projectRoot) {
        if (pendingFile == null || projectRoot == null) return@LaunchedEffect
        val forceEditor = appState.pendingFileForceEditor
        AppContainer.appStateHolder.consumePendingFile()
        // Relative paths are the agent's, so they resolve from where it ran (a
        // worktree chat's cwd is not the project folder), like desktop.
        val cwd = appState.selectedSessionId?.let { AppContainer.sessionStateHolder.views.value[it]?.sessionCwd }?.takeIf { it.isNotBlank() }
        val line = parseLineSuffix(pendingFile)
        var target = resolveFilePath(pendingFile, cwd ?: projectRoot)
        // A bare name only resolves when the file sits in the project root, so look it up.
        if (isBareFileName(pendingFile) && !pendingFile.startsWith("~")) {
            val name = stripLineSuffix(pendingFile.trim())
            val found = runCatching { AppContainer.fsRepository.searchFiles(projectRoot, name, 20, false) }.getOrDefault(emptyList())
                .filter { !it.is_dir && it.absolute_path.substringAfterLast('/').equals(name, ignoreCase = true) }
                .map { it.absolute_path }
            if (target !in found) closestMatch(cwd ?: projectRoot, found)?.let { target = it }
        }
        if (filePreviewKind(target.substringAfterLast('/')) == FilePreviewKind.HTML && !forceEditor) {
            pendingHtmlChoice = target to true
        } else {
            selectFile(target, null, fromLink = true, line = line, forceEditor = forceEditor)
        }
    }

    // File preview view.
    val sel = selectedPath
    if (sel != null) {
        val fileName = sel.split("/").lastOrNull() ?: sel
        val kind = filePreviewKind(fileName)
        val block = if (kind == FilePreviewKind.BLOCKED) getFilePreviewBlock(fileName) else null
        val details = if (kind == FilePreviewKind.DETAILS) fileDetails(fileName, selectedSize) else null
        val previewContext = LocalContext.current
        val relativePath = projectRoot?.let { root ->
            val normalizedRoot = root.trimEnd('/')
            if (sel.startsWith("$normalizedRoot/")) sel.removePrefix("$normalizedRoot/") else sel
        } ?: sel
        val subtitle = listOfNotNull(project?.name?.takeIf { it.isNotBlank() }, relativePath).joinToString(" · ")
        Box(modifier = Modifier.fillMaxSize().background(NewTheme.Black)) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 16.dp),
            ) {
                when {
                    kind == FilePreviewKind.BLOCKED && block != null -> Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = PageHeaderHeight)
                            .clip(RoundedCornerShape(NewTheme.CardRadius))
                            .background(NewTheme.Card)
                            .padding(20.dp),
                    ) {
                        Text(block.title, color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold)
                        Text(block.message, color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.padding(top = 8.dp))
                    }
                    kind == FilePreviewKind.VIDEO || kind == FilePreviewKind.AUDIO -> when {
                        fileLoading && mediaUrl == null -> LoadingState("Loading media…", Modifier.fillMaxSize())
                        fileError != null && mediaUrl == null -> EmptyView(title = "Failed to load media", description = fileError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                        mediaUrl != null -> MediaPreview(url = mediaUrl!!, isVideo = kind == FilePreviewKind.VIDEO, modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight))
                        else -> LoadingState("Loading media…", Modifier.fillMaxSize())
                    }
                    kind == FilePreviewKind.PDF -> when {
                        fileLoading && fileBytes == null -> LoadingState("Loading PDF…", Modifier.fillMaxSize())
                        fileError != null && fileBytes == null -> EmptyView(title = "Failed to load PDF", description = fileError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                        fileBytes != null -> {
                            val pdfBytes = fileBytes!!
                            PdfPreview(
                                bytes = pdfBytes,
                                fileName = fileName,
                                modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight),
                                onOpenExternal = {
                                    scope.launch {
                                        val uri = withContext(Dispatchers.IO) {
                                            writeExternalCopy(previewContext, fileName, pdfBytes)
                                        }
                                        openExternal(previewContext, fileName, uri)
                                    }
                                },
                            )
                        }
                        else -> LoadingState("Loading PDF…", Modifier.fillMaxSize())
                    }
                    kind == FilePreviewKind.DETAILS && details != null -> FileDetailsCard(
                        details = details,
                        fileName = fileName,
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(top = PageHeaderHeight)
                            .clip(RoundedCornerShape(NewTheme.CardRadius))
                            .background(NewTheme.Card)
                            .padding(20.dp),
                        onOpenExternal = {
                            scope.launch {
                                // Opaque files have no preview bytes, so fetch
                                // them now (raw endpoint, media URL as
                                // fallback for files too large for it).
                                val uri = withContext(Dispatchers.IO) {
                                    runCatching {
                                        AppContainer.fsRepository.readFileRaw(sel)
                                    }.mapCatching { writeExternalCopy(previewContext, fileName, it) }
                                        .getOrElse {
                                            Uri.parse(AppContainer.consoleApi.fsMediaUrl(sel))
                                        }
                                }
                                openExternal(previewContext, fileName, uri)
                            }
                        },
                    )
                    kind == FilePreviewKind.HTML && !htmlAsEditor -> {
                        val uriHandler = LocalUriHandler.current
                        HtmlPreviewWebView(
                            path = sel,
                            modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight),
                            onExternalUrl = { handOffToSystem(uriHandler, it) },
                        )
                    }
                    kind == FilePreviewKind.IMAGE -> when {
                        fileLoading && fileBytes == null -> LoadingState("Loading image…", Modifier.fillMaxSize())
                        fileError != null && fileBytes == null -> EmptyView(title = "Failed to load image", description = fileError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                        else -> Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .padding(top = PageHeaderHeight)
                                .padding(16.dp),
                            contentAlignment = Alignment.Center,
                        ) {
                            fileBytes?.let { bytes ->
                                val context = LocalContext.current
                                AsyncImage(
                                    model = ImageRequest.Builder(context)
                                        .data(bytes)
                                        .decoderFactory(SvgDecoder.Factory())
                                        .build(),
                                    contentDescription = fileName,
                                    contentScale = ContentScale.Fit,
                                    modifier = Modifier.fillMaxSize().clickable { zoomImage = bytes },
                                )
                            }
                        }
                    }
                    fileLoading && fileContent == null -> LoadingState("Loading file…", Modifier.fillMaxSize())
                    fileError != null && fileContent == null -> EmptyView(title = "Failed to load file", description = fileError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = Modifier.fillMaxSize())
                    else -> {
                        val content = fileContent ?: ""
                        if (isMarkdownPath(sel)) {
                            // The document scrolls under the header on purpose:
                            // its scrim fades the text out as it leaves instead
                            // of cutting it off at a hard edge. The top padding
                            // lives inside the scroll so the first line still
                            // starts below the header.
                            Column(
                                modifier = Modifier
                                    .fillMaxSize()
                                    .verticalScroll2()
                                    .padding(top = PageHeaderHeight)
                                    .padding(4.dp),
                            ) {
                                CustomMarkdown(content = content)
                            }
                        } else {
                            // The code viewer scrolls itself and cannot pad its
                            // own content, so it starts below the header.
                            Box(modifier = Modifier.fillMaxSize().padding(top = PageHeaderHeight)) {
                                CodePreview(content = content, path = sel, line = targetLine)
                            }
                        }
                    }
                }
            }
            val headerUriHandler = LocalUriHandler.current
            var htmlMenuOpen by remember { mutableStateOf(false) }
            PageHeader(
                title = fileName,
                subtitle = subtitle,
                blurred = true,
                backgroundColor = NewTheme.Black,
                onBack = { openedFromLink = false; selectedPath = null },
                modifier = Modifier.align(Alignment.TopCenter),
                actions = if (kind == FilePreviewKind.HTML) {
                    {
                        Box {
                            IconButton(onClick = { htmlMenuOpen = true }) {
                                Icon(
                                    TablerIcons.Outline.DotsVertical,
                                    contentDescription = "HTML options",
                                    tint = NewTheme.TextPrimary,
                                    modifier = Modifier.size(20.dp),
                                )
                            }
                            OverflowMenu(expanded = htmlMenuOpen, onDismissRequest = { htmlMenuOpen = false }) {
                                if (htmlAsEditor) {
                                    OverflowMenuItem(
                                        label = "Open in browser",
                                        icon = TablerIcons.Outline.Globe,
                                        onClick = {
                                            htmlMenuOpen = false
                                            htmlAsEditor = false
                                        },
                                    )
                                } else {
                                    OverflowMenuItem(
                                        label = "Open in code editor",
                                        icon = TablerIcons.Outline.Code,
                                        onClick = {
                                            htmlMenuOpen = false
                                            htmlAsEditor = true
                                            if (fileContent == null) loadTextContent(sel)
                                        },
                                    )
                                }
                                OverflowMenuItem(
                                    label = "Open externally",
                                    icon = TablerIcons.Outline.ExternalLink,
                                    onClick = {
                                        htmlMenuOpen = false
                                        handOffToSystem(headerUriHandler, AppContainer.consoleApi.fsHtmlUrl(sel))
                                    },
                                )
                            }
                        }
                    }
                } else null,
            )
            zoomImage?.let { bytes ->
                ImagePreviewDialog(image = bytes, onDismiss = { zoomImage = null })
            }
        }
        return
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Black)) {
        PageHeader(
            title = "Files",
            subtitle = project?.name,
            blurred = true,
            backgroundColor = NewTheme.Black,
            onBack = onBack,
            actions = {
                HeaderIconButton(TablerIcons.Outline.Refresh, "Refresh", onClick = {
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
                        gitBadges = loadGitBadges(projectRoot)
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
                                TreeRowEntry(
                                    entry = r,
                                    depth = 0,
                                    selected = false,
                                    expanded = false,
                                    badge = gitBadges[r.path],
                                    onPressDir = { toggleDir(r.path) },
                                    onPressFile = { selectFile(r.path, null) },
                                    onOpenHtmlEditor = { selectFile(r.path, null, forceEditor = true) },
                                )
                            }
                        }
                    }
                }
                else -> LazyColumn(modifier = Modifier.fillMaxSize()) {
                    items(flattenTree(entries, expanded, childrenByPath, loadingDirs), key = { it.key }) { row ->
                        when (row.kind) {
                            TreeKind.Loading -> Row(modifier = Modifier.fillMaxWidth().padding(start = (16 + row.depth * 18).dp).padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                                CircularProgressIndicator(color = NewTheme.TextMuted, strokeWidth = 2.dp, modifier = Modifier.size(14.dp))
                                Text("Loading…", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(start = 8.dp))
                            }
                            TreeKind.Empty -> Text("(empty)", color = NewTheme.TextMuted, fontSize = 13.sp, fontStyle = androidx.compose.ui.text.font.FontStyle.Italic, modifier = Modifier.padding(start = (16 + row.depth * 18).dp, top = 2.dp, bottom = 8.dp))
                            TreeKind.Entry -> {
                                val e = row.entry!!
                                TreeRowEntry(
                                    entry = e,
                                    depth = row.depth,
                                    selected = selectedPath == e.path,
                                    expanded = expanded.contains(e.path),
                                    badge = gitBadges[e.path],
                                    onPressDir = { toggleDir(e.path) },
                                    onPressFile = { selectFile(e.path, e.size) },
                                    onOpenHtmlEditor = { selectFile(e.path, e.size, forceEditor = true) },
                                )
                            }
                        }
                    }
                }
            }
        }
        val choice = pendingHtmlChoice
        if (choice != null) {
            val (choicePath, fromLink) = choice
            val choiceUriHandler = LocalUriHandler.current
            AlertDialog(
                onDismissRequest = { pendingHtmlChoice = null },
                containerColor = NewTheme.Raised,
                title = {
                    Text(
                        choicePath.substringAfterLast('/'),
                        color = NewTheme.TextPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                    )
                },
                text = {
                    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        OverflowMenuItem(
                            label = "Open in browser",
                            icon = TablerIcons.Outline.Globe,
                            onClick = {
                                pendingHtmlChoice = null
                                selectFile(choicePath, null, fromLink = fromLink, forceEditor = false)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open in code editor",
                            icon = TablerIcons.Outline.Code,
                            onClick = {
                                pendingHtmlChoice = null
                                selectFile(choicePath, null, fromLink = fromLink, forceEditor = true)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open externally",
                            icon = TablerIcons.Outline.ExternalLink,
                            onClick = {
                                pendingHtmlChoice = null
                                handOffToSystem(choiceUriHandler, AppContainer.consoleApi.fsHtmlUrl(choicePath))
                            },
                        )
                    }
                },
                confirmButton = {
                    TextButton(onClick = { pendingHtmlChoice = null }) {
                        Text("Cancel", color = NewTheme.TextSecondary)
                    }
                },
            )
        }
        @Suppress("UNUSED_EXPRESSION")
        fsState
    }
}

/**
 * The git badge of every changed path under [root], and of every folder above
 * one — the rules come from the shared crate, so the phone and the desktop
 * badge the same file the same way.
 *
 * Empty when the folder is not a repository or the server cannot be reached:
 * the tree then shows no badges rather than wrong ones.
 */
private suspend fun loadGitBadges(root: String?): Map<String, GitBadgeInfo> = withContext(Dispatchers.IO) {
    val repo = root ?: return@withContext emptyMap()
    try {
        val status = AppContainer.consoleApi.getGitStatus(repo) ?: return@withContext emptyMap()
        val files = status.files
        gitBadges(files.map { it.path }, files.map { it.status })
    } catch (_: Exception) {
        emptyMap()
    }
}

/** Opaque files get a metadata card instead of the dead-end panel. */
@Composable
private fun FileDetailsCard(details: FileDetails, fileName: String, modifier: Modifier, onOpenExternal: () -> Unit) {
    Column(modifier = modifier) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            FileIcon(filename = fileName, sizeDp = 40)
            Column(modifier = Modifier.padding(start = 16.dp)) {
                Text(details.fileName, color = NewTheme.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(details.label, color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(top = 2.dp))
            }
        }
        if (details.fields.isNotEmpty()) {
            Column(modifier = Modifier.padding(top = 16.dp)) {
                for ((label, value) in details.fields) {
                    Row(modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp)) {
                        Text(label, color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.width(96.dp))
                        Text(value, color = NewTheme.TextSecondary, fontSize = 13.sp)
                    }
                }
            }
        }
        PreviewAction(text = "Open externally", icon = TablerIcons.Outline.ExternalLink, onClick = onOpenExternal, modifier = Modifier.padding(top = 16.dp))
    }
}

@Composable
private fun PreviewAction(text: String, icon: androidx.compose.ui.graphics.vector.ImageVector, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(10.dp))
            .background(NewTheme.Raised)
            .clickable(onClick = onClick)
            .padding(vertical = 12.dp),
        horizontalArrangement = Arrangement.Center,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(16.dp))
        Text(text, color = NewTheme.TextSecondary, fontSize = 14.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 8.dp))
    }
}

/** Streams the authenticated media URL; ExoPlayer keeps range requests so seeking works. */
@Composable
private fun MediaPreview(url: String, isVideo: Boolean, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val player = remember(url) {
        ExoPlayer.Builder(context).build().apply {
            setMediaItem(MediaItem.fromUri(url))
            prepare()
            if (isVideo) playWhenReady = true
        }
    }
    DisposableEffect(player) {
        onDispose { player.release() }
    }
    Box(modifier = modifier.background(Color.Black)) {
        AndroidView(
            factory = { ctx ->
                PlayerView(ctx).apply {
                    this.player = player
                    useController = true
                    resizeMode = androidx.media3.ui.AspectRatioFrameLayout.RESIZE_MODE_FIT
                }
            },
            modifier = Modifier.fillMaxSize(),
        )
        if (!isVideo) {
            Icon(
                TablerIcons.Outline.PlayerPlay,
                contentDescription = null,
                tint = Color.White.copy(alpha = 0.5f),
                modifier = Modifier.align(Alignment.Center).size(64.dp),
            )
        }
    }
}

private const val PDF_PREVIEW_MAX_BYTES = 200L * 1024 * 1024

/** Renders PDF pages with the framework PdfRenderer over bytes from the raw endpoint. */
@Composable
private fun PdfPreview(bytes: ByteArray, fileName: String, modifier: Modifier = Modifier, onOpenExternal: () -> Unit = {}) {
    if (bytes.size > PDF_PREVIEW_MAX_BYTES) {
        Column(
            modifier = modifier.padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Text("This PDF is too large to preview here.", color = NewTheme.TextSecondary, fontSize = 14.sp)
            Text("Open it in another app instead.", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(top = 4.dp))
            PreviewAction(text = "Open externally", icon = TablerIcons.Outline.ExternalLink, onClick = onOpenExternal, modifier = Modifier.padding(top = 16.dp))
        }
        return
    }
    val context = LocalContext.current
    var loadError by remember(bytes) { mutableStateOf<String?>(null) }
    val renderer by produceState<PdfRenderer?>(initialValue = null, bytes) {
        value = withContext(Dispatchers.IO) {
            runCatching {
                val dir = File(context.cacheDir, "file_previews").apply { mkdirs() }
                val file = File(dir, "preview-${System.nanoTime()}.pdf")
                try {
                    FileOutputStream(file).use { it.write(bytes) }
                    val pfd = ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY)
                    try {
                        PdfRenderer(pfd)
                    } catch (e: Exception) {
                        pfd.close()
                        throw e
                    }
                } finally {
                    file.delete()
                }
            }.onFailure { loadError = it.message ?: "Couldn't render this PDF." }.getOrNull()
        }
    }
    val pdf = renderer
    if (pdf == null) {
        if (loadError != null) {
            EmptyView(title = "Couldn't render this PDF", description = loadError, icon = TablerIcons.Outline.AlertTriangle, iconTint = NewTheme.Danger, modifier = modifier)
        } else {
            LoadingState("Loading PDF…", modifier)
        }
        return
    }
    if (pdf.pageCount == 0) {
        EmptyView(title = "Empty PDF", description = "This file has no pages.", icon = TablerIcons.Outline.FileTypePdf, modifier = modifier)
        return
    }
    DisposableEffect(pdf) {
        onDispose { pdf.close() }
    }
    Column(
        modifier = modifier.verticalScroll2().padding(horizontal = 8.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        for (pageIndex in 0 until pdf.pageCount) {
            val pageBitmap = remember(pdf, pageIndex) {
                val page = pdf.openPage(pageIndex)
                try {
                    val width = page.width
                    val height = (page.height.toFloat() * width / page.width).toInt()
                    Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888).also { bmp ->
                        page.render(bmp, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                    }
                } finally {
                    page.close()
                }
            }
            Image(
                bitmap = pageBitmap.asImageBitmap(),
                contentDescription = "$fileName page ${pageIndex + 1}",
                contentScale = ContentScale.FillWidth,
                modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(4.dp)),
            )
        }
    }
}

/** Copies bytes into the shared cache so FileProvider can hand them to another app. */
private fun writeExternalCopy(context: android.content.Context, fileName: String, bytes: ByteArray): Uri {
    val dir = File(context.cacheDir, "file_previews").apply { mkdirs() }
    val safeName = fileName.replace(Regex("[^A-Za-z0-9._-]"), "_")
    val file = File(dir, safeName)
    FileOutputStream(file).use { it.write(bytes) }
    return FileProvider.getUriForFile(context, "${context.packageName}.updates", file)
}

private fun openExternal(context: android.content.Context, fileName: String, uri: Uri) {
    val mime = mimeTypeForName(fileName) ?: "*/*"
    val intent = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(uri, mime)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    }
    runCatching { context.startActivity(Intent.createChooser(intent, "Open $fileName")) }
}

private enum class TreeKind { Entry, Loading, Empty }
private data class FlatRow(val key: String, val kind: TreeKind, val entry: FsTreeEntry? = null, val depth: Int)

private fun flattenTree(roots: List<FsTreeEntry>, expanded: Set<String>, children: Map<String, List<FsTreeEntry>>, loading: Set<String>): List<FlatRow> {
    val out = mutableListOf<FlatRow>()
    fun visit(list: List<FsTreeEntry>, depth: Int) {
        val sorted = list.sortedWith(compareBy({ !it.is_dir }, { it.name.lowercase() }))
        for (e in sorted) {
            out.add(FlatRow("row:${e.path}", TreeKind.Entry, e, depth))
            if (e.is_dir && expanded.contains(e.path)) {
                val kids = children[e.path]
                when {
                    // Only a fetch that is still running shows a spinner. An
                    // expanded folder with no children and no fetch in flight
                    // is one whose load failed, and a spinner there would spin
                    // forever.
                    kids == null -> if (loading.contains(e.path)) {
                        out.add(FlatRow("loading:${e.path}", TreeKind.Loading, null, depth + 1))
                    }
                    kids.isEmpty() -> out.add(FlatRow("empty:${e.path}", TreeKind.Empty, null, depth + 1))
                    else -> visit(kids, depth + 1)
                }
            }
        }
    }
    visit(roots, 0)
    return out
}

@Composable
private fun TreeRowEntry(
    entry: FsTreeEntry,
    depth: Int,
    selected: Boolean,
    expanded: Boolean,
    badge: GitBadgeInfo?,
    onPressDir: () -> Unit,
    onPressFile: () -> Unit,
    onOpenHtmlEditor: () -> Unit = onPressFile,
) {
    val isHtml = !entry.is_dir && isHtmlFileName(entry.name)
    var htmlMenuOpen by remember { mutableStateOf(false) }
    val uriHandler = LocalUriHandler.current
    Box(modifier = Modifier.fillMaxWidth()) {
    Row(
        modifier = Modifier.fillMaxWidth()
            .background(if (selected) Color.White.copy(alpha = 0.06f) else Color.Transparent)
            .clickable {
                when {
                    entry.is_dir -> onPressDir()
                    isHtml -> htmlMenuOpen = true
                    else -> onPressFile()
                }
            }
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
                    // A folder carries the strongest badge beneath it, so the
                    // tint says what changed inside it.
                    tint = badge?.let { gitRoleColor(it.role) } ?: NewTheme.TextPrimary,
                    modifier = Modifier.size(20.dp),
                )
            } else {
                FileIcon(filename = entry.name, sizeDp = 20)
            }
        }
        Text(
            entry.name,
            // The name carries the badge too, so a change is visible on the
            // label and not only on the icon.
            color = if (selected) NewTheme.TextPrimary else badge?.let { gitRoleColor(it.role) } ?: NewTheme.TextPrimary,
            fontSize = 16.sp,
            fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f).padding(start = 12.dp),
        )
        // The letter sits after the name, so a long name truncates instead of
        // pushing the badge off the row.
        if (badge != null) {
            Text(badge.label, color = gitRoleColor(badge.role), fontSize = 12.sp, fontWeight = FontWeight.Bold, modifier = Modifier.width(14.dp))
        }
    }
    if (isHtml) {
        OverflowMenu(expanded = htmlMenuOpen, onDismissRequest = { htmlMenuOpen = false }) {
            OverflowMenuItem(
                label = "Open in browser",
                icon = TablerIcons.Outline.Globe,
                onClick = {
                    htmlMenuOpen = false
                    onPressFile()
                },
            )
            OverflowMenuItem(
                label = "Open in code editor",
                icon = TablerIcons.Outline.Code,
                onClick = {
                    htmlMenuOpen = false
                    onOpenHtmlEditor()
                },
            )
            OverflowMenuItem(
                label = "Open externally",
                icon = TablerIcons.Outline.ExternalLink,
                onClick = {
                    htmlMenuOpen = false
                    handOffToSystem(uriHandler, AppContainer.consoleApi.fsHtmlUrl(entry.path))
                },
            )
        }
    }
    }
}

@Composable
private fun CodePreview(content: String, path: String?, line: Int?) {
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
        jumpToLine = line,
    )
}

@Composable
private fun Modifier.verticalScroll2(): Modifier = this.verticalScroll(androidx.compose.foundation.rememberScrollState())
@Composable
private fun Modifier.hScroll2(): Modifier = this.horizontalScroll(androidx.compose.foundation.rememberScrollState())

/** What system back does from a file preview. */
enum class PreviewBack { ToPreviousScreen, ToFileTree }

/** A file opened from a chat link returns to that chat; one opened from the tree returns to the tree. */
fun previewBackAction(openedFromLink: Boolean): PreviewBack =
    if (openedFromLink) PreviewBack.ToPreviousScreen else PreviewBack.ToFileTree
