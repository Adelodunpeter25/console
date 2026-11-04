package com.console.mobile.feature.scripts

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
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
import com.console.mobile.data.repo.ProjectScriptsState
import com.console.mobile.data.repo.ScriptRunView
import com.console.mobile.data.repo.ScriptsRepository
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.TintPill
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import console.v1.ProjectScript
import console.v1.ScriptRun
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerPlay
import io.github.lyxnx.compose.ui.tablericons.outline.PlayerStop
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import io.github.lyxnx.compose.ui.tablericons.outline.Rocket
import kotlinx.coroutines.launch

/**
 * The chat's project run scripts (`console.toml` `[scripts]`) as a bottom
 * sheet: every script with its latest status, a run/stop control, and the
 * run's live output. Reached from the chat overflow menu, beside Ports and
 * Devices.
 *
 * The sheet is a view over [ScriptsRepository]; a run started here keeps
 * streaming (and keeps the header's running indicator lit) after the sheet
 * closes.
 */
@Composable
fun ScriptsSheet(
    projectId: String,
    cwd: String?,
    onOpenUrl: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val repo = AppContainer.scriptsRepository
    val states by repo.states.collectAsStateWithLifecycle()
    val runs by repo.runs.collectAsStateWithLifecycle()
    val state: ProjectScriptsState =
        states[ScriptsRepository.projectKey(projectId, cwd)] ?: ProjectScriptsState()
    val scope = rememberCoroutineScope()

    var expanded by remember { mutableStateOf(emptySet<String>()) }
    var notice by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(projectId, cwd) { repo.refresh(projectId, cwd) }

    BaseSheet(onDismiss = onDismiss, title = "Run scripts", scrollable = false) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                text = when {
                    state.source == "missing" -> "No console.toml found for this project"
                    state.source.isNotBlank() -> "From ${state.source}"
                    else -> "Project run scripts"
                },
                color = NewTheme.TextMuted,
                fontSize = 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f).padding(start = 4.dp),
            )
            IconButton(
                onClick = { scope.launch { repo.refresh(projectId, cwd) } },
                modifier = Modifier.size(40.dp),
            ) {
                if (state.loading) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(14.dp),
                        strokeWidth = 2.dp,
                        color = NewTheme.TextPrimary,
                    )
                } else {
                    Icon(
                        TablerIcons.Outline.Refresh,
                        contentDescription = "Refresh scripts",
                        tint = NewTheme.TextSecondary,
                        modifier = Modifier.size(18.dp),
                    )
                }
            }
        }

        val banner = notice ?: state.error
        if (banner != null && state.scripts.isNotEmpty()) {
            Banner(banner, modifier = Modifier.padding(top = 4.dp, bottom = 4.dp))
        }

        val hasScripts = state.scripts.isNotEmpty()
        when {
            state.loading && !hasScripts -> LoadingState("Loading run scripts")

            state.error != null && !hasScripts && !state.loading -> EmptyView(
                title = "Couldn't load run scripts",
                description = state.error,
                icon = TablerIcons.Outline.AlertTriangle,
                iconTint = NewTheme.Danger,
                action = {
                    ActionButton(
                        text = "Retry",
                        onClick = { scope.launch { repo.refresh(projectId, cwd) } },
                    )
                },
            )

            !hasScripts -> EmptyView(
                title = "No run scripts",
                description = "Add a [scripts] section to console.toml and its commands show up here.",
                icon = TablerIcons.Outline.Rocket,
            )

            else -> LazyColumn(
                modifier = Modifier.fillMaxWidth().weight(1f, fill = false),
                contentPadding = PaddingValues(top = 4.dp, bottom = 8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(state.scripts, key = { it.id }) { script ->
                    val view = runs[ScriptsRepository.runKey(projectId, cwd, script.id)]
                        ?: ScriptRunView()
                    ScriptRowItem(
                        script = script,
                        view = view,
                        expanded = script.id in expanded,
                        onToggle = {
                            expanded = if (script.id in expanded) expanded - script.id else expanded + script.id
                        },
                        onRun = { repo.run(projectId, cwd, script.id) },
                        onStop = { repo.stop(projectId, cwd, script.id) },
                        onOpenPort = { port ->
                            scope.launch {
                                notice = null
                                val url = AppContainer.portsRepository.openUrl(port)
                                if (url != null) {
                                    onOpenUrl(url)
                                } else {
                                    notice = "Port $port has no reachable URL"
                                }
                            }
                        },
                    )
                }
            }
        }
    }
}

/** Status label + tint for a run's latest status, or null when never run. */
internal fun scriptStatus(run: ScriptRun?): Pair<String, Color>? = when (run?.status) {
    null, "" -> null
    "running" -> "Running" to NewTheme.Accent
    "succeeded" -> "Succeeded" to NewTheme.Success
    "failed" -> {
        val exit = run.exit_code?.let { " · exit $it" }.orEmpty()
        "Failed$exit" to NewTheme.Danger
    }
    "stopped" -> "Stopped" to NewTheme.TextMuted
    else -> run.status.replaceFirstChar { it.uppercase() } to NewTheme.TextMuted
}

/** How much of a run's output the sheet shows: the tail, newest last. */
private const val OUTPUT_TAIL_LINES = 60

/**
 * One script: label + command, its latest-status pill, a run/stop control,
 * the output tail once expanded, and an "open in browser" shortcut when the
 * output announces a dev-server port.
 */
@Composable
private fun ScriptRowItem(
    script: ProjectScript,
    view: ScriptRunView,
    expanded: Boolean,
    onToggle: () -> Unit,
    onRun: () -> Unit,
    onStop: () -> Unit,
    onOpenPort: (Int) -> Unit,
) {
    val status = scriptStatus(view.run)
    val port = remember(view.output) { ScriptsRepository.portInOutput(view.output) }

    Column(
        modifier = Modifier.fillMaxWidth()
            .clip(RoundedCornerShape(NewTheme.CardRadius))
            .background(NewTheme.Card)
            .clickable(enabled = view.output.isNotBlank(), onClick = onToggle),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(modifier = Modifier.weight(1f).padding(end = 10.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        script.label.ifBlank { script.id },
                        color = NewTheme.TextPrimary,
                        fontSize = 16.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    if (status != null) {
                        Spacer(Modifier.size(8.dp))
                        TintPill(status.first, status.second)
                    }
                }
                Text(
                    script.command,
                    color = NewTheme.TextMuted,
                    fontSize = 12.sp,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
            when {
                view.starting -> CircularProgressIndicator(
                    color = NewTheme.TextSecondary,
                    strokeWidth = 2.dp,
                    modifier = Modifier.size(16.dp),
                )
                view.isRunning -> ActionButton(
                    text = "Stop",
                    onClick = onStop,
                    kind = ActionButtonKind.Danger,
                    icon = TablerIcons.Outline.PlayerStop,
                    compact = true,
                    surface = NewTheme.Raised,
                )
                else -> ActionButton(
                    text = "Run",
                    onClick = onRun,
                    icon = TablerIcons.Outline.PlayerPlay,
                    compact = true,
                    surface = NewTheme.Raised,
                )
            }
        }

        val canOpenPort = view.isRunning && port != null
        if (canOpenPort) {
            ActionButton(
                text = "Open localhost:$port",
                onClick = { onOpenPort(port) },
                compact = true,
                surface = NewTheme.Raised,
                modifier = Modifier.padding(start = 14.dp, end = 14.dp, bottom = 12.dp),
            )
        }

        if (expanded && view.output.isNotBlank()) {
            val tail = remember(view.output) {
                view.output.trimEnd().lines().takeLast(OUTPUT_TAIL_LINES).joinToString("\n")
            }
            Box(
                modifier = Modifier.fillMaxWidth()
                    .padding(start = 14.dp, end = 14.dp, bottom = 12.dp)
                    .clip(RoundedCornerShape(NewTheme.FieldRadius))
                    .background(NewTheme.Field)
                    .padding(10.dp),
            ) {
                Text(
                    tail,
                    color = NewTheme.TextSecondary,
                    fontFamily = ConsoleMonoFamily,
                    fontSize = 11.sp,
                    lineHeight = 15.sp,
                )
            }
        }
    }
}
