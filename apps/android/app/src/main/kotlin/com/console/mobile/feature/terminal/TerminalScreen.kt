package com.console.mobile.feature.terminal

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
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
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.FolderOpen
import io.github.lyxnx.compose.ui.tablericons.outline.Keyboard
import io.github.lyxnx.compose.ui.tablericons.outline.KeyboardOff
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import com.console.mobile.AppContainer
import console.v1.ProjectInfo
import com.console.mobile.data.store.TerminalStatus
import com.console.mobile.feature.terminal.native.NativeTerminalView
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import com.console.mobile.ui.components.common.new.CircleIconButton
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.Section
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.theme.NewTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.height
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight

private data class ExtraKey(val label: String, val bytes: String)

private val EXTRA_KEYS = listOf(
    ExtraKey("Esc", "\u001B"),
    ExtraKey("Tab", "\t"),
    ExtraKey("↑", "\u001BA"),
    ExtraKey("↓", "\u001BB"),
    ExtraKey("←", "\u001BD"),
    ExtraKey("→", "\u001BC"),
    ExtraKey("Ctrl-C", "\u0003"),
)

/**
 * Port of screens/terminal/terminal-screen.tsx + useTerminalScreen + extra-keys-bar.
 * Ghostty native surface reuses modules/console-terminal's JNI (libghostty-vt +
 * TerminalCanvasView) verbatim via NativeTerminalView, hosted through AndroidView.
 * Leaving the screen keeps the PTY alive (no kill on dispose).
 */
@Composable
fun TerminalScreen(onBack: () -> Unit) {
    val scope = rememberCoroutineScope()
    val appState by AppContainer.appStateHolder.state.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val terminals by AppContainer.terminalStateHolder.terminals.collectAsStateWithLifecycle()
    val buffers by AppContainer.terminalStateHolder.buffers.collectAsStateWithLifecycle()

    val project = appState.selectedProjectId?.let { id -> projectState.projects.firstOrNull { it.id == id } }
    val needsProjectPick = project == null && projectState.projects.isNotEmpty()
    var terminalId by remember(project?.id, project?.path) { mutableStateOf<String?>(null) }
    var spawnError by remember(project?.id, project?.path) { mutableStateOf<String?>(null) }

    // Spawn or reuse on entry (80×24 default; resize follows surface — PTY reflows).
    LaunchedEffect(project?.id, project?.path) {
        val p = project ?: return@LaunchedEffect
        terminalId = null
        spawnError = null
        try {
            val live = withContext(Dispatchers.IO) {
                AppContainer.terminalStateHolder.findLive(p.id, p.path)
            }
            if (live != null) {
                terminalId = live
                return@LaunchedEffect
            }
            val spawned = withContext(Dispatchers.IO) {
                AppContainer.terminalRepository.openTerminal(projectId = p.id, cwd = p.path, cols = 80, rows = 24)
            }
            terminalId = spawned.id
        } catch (e: Exception) {
            spawnError = e.message ?: e.toString()
        }
    }

    val term = terminalId?.let { terminals[it] }
    val buffer = terminalId?.let { buffers[it] } ?: ""
    val isRunning = term?.status == TerminalStatus.Running || term?.status == TerminalStatus.Spawning

    fun killAndRespawn() {
        val p = project ?: return
        val id = terminalId ?: return
        terminalId = null
        spawnError = null
        scope.launch {
            withContext(Dispatchers.IO) {
                try { AppContainer.terminalRepository.kill(id) } catch (_: Exception) {}
            }
            try {
                val spawned = withContext(Dispatchers.IO) {
                    AppContainer.terminalRepository.openTerminal(projectId = p.id, cwd = p.path, cols = 80, rows = 24)
                }
                terminalId = spawned.id
            } catch (e: Exception) {
                spawnError = e.message ?: e.toString()
            }
        }
    }

    val statusBanner = when {
        spawnError != null -> "Failed to start terminal: $spawnError"
        project == null -> if (projectState.projects.isEmpty()) "No projects yet — add a project first." else null
        term?.status == TerminalStatus.Exited -> "Session ended."
        term?.status == TerminalStatus.Error -> "Session error: ${term.error ?: "unknown"}"
        else -> null
    }

    Column(modifier = Modifier.fillMaxSize().background(NewTheme.Background).imePadding()) {
        PageHeader(
            title = "Terminal",
            onBack = onBack,
            actions = if (term != null) {
                {
                    CircleIconButton(TablerIcons.Outline.Trash, "Restart shell", onClick = ::killAndRespawn)
                }
            } else null,
        )
        if (statusBanner != null) {
            Text(statusBanner, color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(horizontal = 16.dp).padding(bottom = 8.dp))
        }
        when {
            needsProjectPick -> ProjectPicker(
                projects = projectState.projects,
                onSelect = { AppContainer.appStateHolder.setSelectedProjectId(it) },
            )
            project == null -> EmptyView(title = "No projects yet", description = "Add a project folder in Settings → Projects to open a shell.", icon = TablerIcons.Outline.FolderOpen, modifier = Modifier.fillMaxSize())
            else -> {
                Box(modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp)) {
                    Box(modifier = Modifier.fillMaxSize().background(Color.Black)) {
                        if (terminalId == null && spawnError == null) {
                            Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                    CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                                    Text("Starting shell…", color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(top = 8.dp))
                                }
                            }
                        } else {
                            GhosttyTerminalSurface(
                                terminalId = terminalId,
                                buffer = buffer,
                                isRunning = isRunning,
                            )
                        }
                    }
                }
                ExtraKeysBar(
                    onExtraKey = { bytes ->
                        val id = terminalId ?: return@ExtraKeysBar
                        scope.launch { withContext(Dispatchers.IO) { AppContainer.terminalRepository.write(id, bytes) } }
                    },
                )
            }
        }
    }
}

/** Hosts NativeTerminalView (Ghostty JNI renderer) inside Compose. */
@Composable
private fun GhosttyTerminalSurface(terminalId: String?, buffer: String, isRunning: Boolean) {
    val scope = rememberCoroutineScope()
    val terminalIdState = rememberUpdatedState(terminalId)
    AndroidView(
        factory = { ctx ->
            NativeTerminalView(ctx).apply {
                onInput = onInput@{ data ->
                    val id = terminalIdState.value ?: return@onInput
                    scope.launch { withContext(Dispatchers.IO) { AppContainer.terminalRepository.write(id, data) } }
                }
                onResize = onResize@{ cols, rows ->
                    val id = terminalIdState.value ?: return@onResize
                    scope.launch { withContext(Dispatchers.IO) { AppContainer.terminalRepository.resize(id, cols, rows) } }
                }
            }
        },
        update = { view ->
            view.initialBuffer = buffer
        },
        onRelease = { view -> view.cleanup() },
        modifier = Modifier.fillMaxSize(),
    )
    DisposableEffect(Unit) {
        onDispose {}
    }
}

@Composable
private fun ProjectPicker(projects: List<ProjectInfo>, onSelect: (String) -> Unit) {
    Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
        Section("Select a project") {
            projects.forEachIndexed { index, p ->
                Row(
                    modifier = Modifier.fillMaxWidth().clickable { onSelect(p.id) }.padding(horizontal = 18.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(TablerIcons.Outline.Folder, contentDescription = null, tint = NewTheme.TextPrimary, modifier = Modifier.size(24.dp))
                    Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
                        Text(p.name, color = NewTheme.TextPrimary, fontSize = 17.sp)
                        Text(p.path, color = NewTheme.TextMuted, fontSize = 12.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 1.dp))
                    }
                    Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(20.dp))
                }
                if (index < projects.lastIndex) SectionDivider(startInset = 58.dp)
            }
        }
    }
}

/** PTY control strip: extra keys the soft keyboard cannot produce. */
@Composable
private fun ExtraKeysBar(onExtraKey: (String) -> Unit) {
    val keyboard = LocalSoftwareKeyboardController.current
    var kbVisible by remember { mutableStateOf(false) }
    Row(
        modifier = Modifier.fillMaxWidth().background(NewTheme.Background).horizontalScroll(rememberScrollState()).padding(horizontal = 10.dp, vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Box(
            modifier = Modifier.size(38.dp).clip(RoundedCornerShape(NewTheme.ChipRadius)).background(NewTheme.Card)
                .clickable(onClickLabel = "Toggle keyboard") { if (kbVisible) keyboard?.hide() else keyboard?.show(); kbVisible = !kbVisible },
            contentAlignment = Alignment.Center,
        ) {
            Icon(if (kbVisible) TablerIcons.Outline.KeyboardOff else TablerIcons.Outline.Keyboard, contentDescription = "Toggle keyboard", tint = NewTheme.TextSecondary, modifier = Modifier.size(18.dp))
        }
        EXTRA_KEYS.forEach { k -> KeyButton(label = k.label, onClick = { onExtraKey(k.bytes) }) }
    }
}

@Composable
private fun KeyButton(label: String, onClick: () -> Unit) {
    Box(
        modifier = Modifier.height(38.dp).clip(RoundedCornerShape(NewTheme.ChipRadius)).background(NewTheme.Card).clickable(onClick = onClick).padding(horizontal = 14.dp),
        contentAlignment = Alignment.Center,
    ) {
        Text(label, color = NewTheme.TextSecondary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily)
    }
}
