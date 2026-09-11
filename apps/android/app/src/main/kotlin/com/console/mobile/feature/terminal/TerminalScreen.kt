package com.console.mobile.feature.terminal

import android.view.KeyEvent
import android.widget.EditText
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Keyboard
import androidx.compose.material.icons.filled.KeyboardHide
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalSoftwareKeyboardController
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.store.TerminalStatus
import com.console.mobile.data.model.ProjectInfo
import com.console.mobile.ui.components.EmptyState
import com.console.mobile.ui.components.ScreenHeader
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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
 * Ghostty native surface is a Phase-5 follow-up (reuses modules/console-terminal
 * JNI verbatim); meanwhile: PTY-backed scrollback + hidden EditText IME input +
 * extra-keys strip. Leaving the screen keeps the PTY alive (no kill on dispose).
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
    var resizeJob by remember { mutableStateOf<Job?>(null) }
    val vScroll = rememberScrollState()
    val hScroll = rememberScrollState()

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

    // Follow tail on new output.
    LaunchedEffect(buffer.length) {
        try { vScroll.animateScrollTo(vScroll.maxValue) } catch (_: Exception) {}
    }

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
        term?.status == TerminalStatus.Error -> "Session error: ${term?.error ?: "unknown"}"
        else -> null
    }

    Column(modifier = Modifier.fillMaxSize().background(ConsoleColors.Background)) {
        ScreenHeader(
            title = "Terminal",
            onBack = onBack,
            actions = if (term != null) {
                {
                    IconButton(onClick = ::killAndRespawn, modifier = Modifier.size(40.dp)) {
                        Icon(Icons.Filled.Delete, contentDescription = "Restart shell", tint = ConsoleColors.Destructive, modifier = Modifier.size(18.dp))
                    }
                }
            } else null,
        )
        if (statusBanner != null) {
            Text(statusBanner, color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(horizontal = 16.dp).padding(bottom = 8.dp))
        }
        when {
            needsProjectPick && project == null -> ProjectPicker(
                projects = projectState.projects,
                onSelect = { AppContainer.appStateHolder.setSelectedProjectId(it) },
            )
            project == null -> EmptyState(title = "No projects yet", description = "Add a project folder in Settings → Projects to open a shell.")
            else -> {
                Box(modifier = Modifier.weight(1f).fillMaxWidth().padding(horizontal = 8.dp)) {
                    // Scrollback (read) + hidden IME input (write).
                    Column(modifier = Modifier.fillMaxSize()) {
                        Box(modifier = Modifier.weight(1f).fillMaxWidth().clip(RoundedCornerShape(8.dp)).background(Color.Black).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp)).padding(8.dp)) {
                            if (terminalId == null && spawnError == null) {
                                Box(modifier = Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
                                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                                        CircularProgressIndicator(color = Color.White, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
                                        Text("Starting shell…", color = ConsoleColors.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 8.dp))
                                    }
                                }
                            } else {
                                Text(
                                    terminalTail(buffer),
                                    color = Color(0xFFE4E4E7),
                                    fontSize = 13.sp,
                                    fontFamily = ConsoleMonoFamily,
                                    lineHeight = 19.sp,
                                    modifier = Modifier.fillMaxSize().verticalScroll(vScroll).horizontalScroll(hScroll),
                                )
                            }
                        }
                        TerminalInputRow(
                            enabled = isRunning && terminalId != null,
                            onInput = { data ->
                                val id = terminalId ?: return@TerminalInputRow
                                scope.launch { withContext(Dispatchers.IO) { AppContainer.terminalRepository.write(id, data) } }
                            },
                        )
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
    @Suppress("UNUSED_EXPRESSION")
    resizeJob
}

private fun terminalTail(buffer: String, maxChars: Int = 60_000): String {
    if (buffer.length <= maxChars) return buffer.ifBlank { "" }
    val cut = buffer.length - maxChars
    val nl = buffer.indexOf('\n', cut)
    return buffer.substring(if (nl == -1) cut else nl + 1)
}

@Composable
private fun ProjectPicker(projects: List<ProjectInfo>, onSelect: (String) -> Unit) {
    Column(modifier = Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 32.dp)) {
        Text("Select a project", color = ConsoleColors.TextPrimary, fontSize = 16.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(vertical = 12.dp))
        projects.forEach { p ->
            Row(
                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp)).clickable { onSelect(p.id) }.padding(14.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Filled.Folder, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(16.dp))
                Column(modifier = Modifier.weight(1f).padding(start = 10.dp)) {
                    Text(p.name, color = ConsoleColors.TextPrimary, fontSize = 14.sp, fontWeight = FontWeight.SemiBold)
                    Text(p.path, color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}

/** Hidden EditText bridged via AndroidView for IME input; echoes bytes to the PTY. */
@Composable
private fun TerminalInputRow(enabled: Boolean, onInput: (String) -> Unit) {
    val keyboard = LocalSoftwareKeyboardController.current
    Row(modifier = Modifier.fillMaxWidth().padding(top = 8.dp), verticalAlignment = Alignment.CenterVertically) {
        AndroidView(
            factory = { ctx ->
                EditText(ctx).apply {
                    hint = "Type here…"
                    setHintTextColor(android.graphics.Color.parseColor("#71717a"))
                    setTextColor(android.graphics.Color.WHITE)
                    setBackgroundColor(android.graphics.Color.TRANSPARENT)
                    textSize = 14f
                    isSingleLine = false
                    imeOptions = android.view.inputmethod.EditorInfo.IME_FLAG_NO_FULLSCREEN
                    inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE
                    setOnKeyListener { _, keyCode, event ->
                        if (event.action == KeyEvent.ACTION_DOWN && keyCode == KeyEvent.KEYCODE_DEL && text.isNullOrEmpty()) {
                            onInput("\u007F")
                            true
                        } else false
                    }
                    addTextChangedListener(object : android.text.TextWatcher {
                        override fun beforeTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {}
                        override fun onTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {
                            if (s == null) return
                            if (c > 0) {
                                val inserted = s.substring(a, a + c).toString()
                                onInput(inserted)
                            }
                        }
                        override fun afterTextChanged(s: android.text.Editable?) {
                            s?.clear()
                        }
                    })
                }
            },
            modifier = Modifier.weight(1f).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(12.dp)).padding(horizontal = 12.dp, vertical = 4.dp),
        )
        IconButton(onClick = { keyboard?.show() }, modifier = Modifier.size(40.dp)) {
            Icon(Icons.Filled.Keyboard, contentDescription = "Show keyboard", tint = ConsoleColors.TextSecondary)
        }
    }
}

/** PTY control strip: extra keys the soft keyboard cannot produce. */
@Composable
private fun ExtraKeysBar(onExtraKey: (String) -> Unit) {
    val keyboard = LocalSoftwareKeyboardController.current
    var kbVisible by remember { mutableStateOf(false) }
    Row(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Background).border(1.dp, ConsoleColors.BorderSubtle).horizontalScroll(rememberScrollState()).padding(horizontal = 8.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = { if (kbVisible) keyboard?.hide() else keyboard?.show(); kbVisible = !kbVisible }, modifier = Modifier.size(32.dp).clip(RoundedCornerShape(6.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, RoundedCornerShape(6.dp))) {
            Icon(if (kbVisible) Icons.Filled.KeyboardHide else Icons.Filled.Keyboard, contentDescription = "Toggle keyboard", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(15.dp))
        }
        EXTRA_KEYS.forEach { k ->
            TextButton2(label = k.label, onClick = { onExtraKey(k.bytes) })
        }
    }
}

@Composable
private fun TextButton2(label: String, onClick: () -> Unit) {
    Box(modifier = Modifier.padding(start = 6.dp).clip(RoundedCornerShape(6.dp)).background(ConsoleColors.Card).border(1.dp, ConsoleColors.Border, RoundedCornerShape(6.dp)).clickable(onClick = onClick).padding(horizontal = 10.dp, vertical = 8.dp), contentAlignment = Alignment.Center) {
        Text(label, color = ConsoleColors.TextSecondary, fontSize = 12.sp, fontFamily = ConsoleMonoFamily)
    }
}
