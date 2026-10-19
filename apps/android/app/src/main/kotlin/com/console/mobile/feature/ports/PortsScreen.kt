package com.console.mobile.feature.ports

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.Banner
import com.console.mobile.ui.components.common.new.EmptyView
import com.console.mobile.ui.components.common.new.InlineTextInput
import com.console.mobile.ui.components.common.new.LoadingState
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.components.common.new.PageHeader
import com.console.mobile.ui.components.common.new.TintPill
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.Globe
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.PlugOff
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import kotlinx.coroutines.launch

/** Privileged ports are not forwardable server-side, so do not offer them. */
private val FORWARDABLE_PORTS = 1024..65535

/**
 * The server's forwarded dev ports and the tunnels this device holds into them.
 *
 * Rows open in the app's own browser: the loopback tunnel when one is up, the
 * server's proxy URL when it is not. Forwarding a new port is one field, and
 * stopping one is a row action.
 */
@Composable
fun PortsScreen(
    onOpenPort: (String) -> Unit,
    modifier: Modifier = Modifier,
    onBack: (() -> Unit)? = null,
) {
    val repo = AppContainer.portsRepository
    val ports by repo.ports.collectAsStateWithLifecycle()
    val locals by repo.locals.collectAsStateWithLifecycle()
    val isLoading by repo.isLoading.collectAsStateWithLifecycle()
    val error by repo.error.collectAsStateWithLifecycle()
    val projectState by AppContainer.projectStateHolder.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    var draft by remember { mutableStateOf("") }
    var draftError by remember { mutableStateOf<String?>(null) }
    var notice by remember { mutableStateOf<String?>(null) }
    var openingPort by remember { mutableStateOf<Int?>(null) }

    LaunchedEffect(Unit) { repo.refresh() }

    val rows = remember(ports, locals) { portRows(ports, locals) }
    val projectNames = remember(projectState.projects) {
        projectState.projects.associate { it.id to it.name }
    }

    val forward: () -> Unit = {
        val port = draft.trim().toIntOrNull()
        when {
            port == null -> draftError = "Enter a port number"
            port !in FORWARDABLE_PORTS -> draftError = "Ports below 1024 can't be forwarded"
            else -> scope.launch {
                draftError = null
                notice = null
                val result = repo.forward(port)
                if (result.isSuccess) {
                    draft = ""
                } else {
                    draftError = result.exceptionOrNull()?.message ?: "Could not forward $port"
                }
            }
        }
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .background(NewTheme.Background),
    ) {
        PageHeader(
            title = "Ports",
            onBack = onBack,
            actions = {
                IconButton(
                    onClick = { scope.launch { repo.refresh() } },
                    modifier = Modifier.size(40.dp),
                ) {
                    if (isLoading) {
                        CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = NewTheme.TextPrimary)
                    } else {
                        Icon(TablerIcons.Outline.Refresh, contentDescription = "Refresh ports", tint = NewTheme.TextSecondary, modifier = Modifier.size(18.dp))
                    }
                }
            },
        )

        ForwardRow(
            value = draft,
            onValueChange = { draft = it.filter(Char::isDigit).take(5); draftError = null },
            error = draftError,
            onForward = forward,
        )

        val banner = notice ?: error
        if (banner != null && rows.isNotEmpty()) {
            Banner(banner, modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp))
        }

        when {
            isLoading && rows.isEmpty() -> LoadingState("Loading ports")

            rows.isEmpty() -> EmptyView(
                title = if (error != null) "Could not load ports" else "No forwarded ports",
                description = error
                    ?: "Ports the backend forwards show up here and open in the app's browser.",
                icon = TablerIcons.Outline.PlugConnected,
                iconTint = if (error != null) NewTheme.Danger else NewTheme.TextMuted,
                action = if (error != null) ({
                    ActionButton(text = "Retry", onClick = { scope.launch { repo.refresh() } })
                }) else null,
            )

            else -> LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(top = 8.dp, bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(rows, key = { it.port }) { row ->
                    PortRowItem(
                        row = row,
                        projectName = row.projectId?.let { projectNames[it] },
                        busy = openingPort == row.port,
                        onOpen = {
                            scope.launch {
                                openingPort = row.port
                                notice = null
                                val url = repo.openUrl(row.port)
                                openingPort = null
                                if (url != null) onOpenPort(url) else notice = "Port ${row.port} has no reachable URL"
                            }
                        },
                        onUnforward = {
                            scope.launch {
                                notice = null
                                val result = repo.unforward(row.port)
                                if (result.isFailure) {
                                    notice = result.exceptionOrNull()?.message ?: "Could not stop forwarding ${row.port}"
                                }
                            }
                        },
                    )
                }
            }
        }
    }
}

/** The one-field "forward another port" control that heads the list. */
@Composable
private fun ForwardRow(
    value: String,
    onValueChange: (String) -> Unit,
    error: String?,
    onForward: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            InlineTextInput(
                value = value,
                onValueChange = onValueChange,
                placeholder = "Forward a port (e.g. 3000)",
                maxLines = 1,
                keyboardType = KeyboardType.Number,
                modifier = Modifier.weight(1f),
            )
            ActionButton(
                text = "Forward",
                onClick = onForward,
                kind = ActionButtonKind.Primary,
                enabled = value.isNotBlank(),
            )
        }
        if (error != null) {
            Text(
                text = error,
                color = NewTheme.Danger,
                fontSize = 12.sp,
                modifier = Modifier.padding(start = 4.dp, top = 6.dp),
            )
        }
    }
}

/** One forwarded port: tap to open, menu to stop forwarding. */
@Composable
private fun PortRowItem(
    row: PortRow,
    projectName: String?,
    busy: Boolean,
    onOpen: () -> Unit,
    onUnforward: () -> Unit,
) {
    var menuOpen by remember { mutableStateOf(false) }

    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp)
            .clip(RoundedCornerShape(NewTheme.FieldRadius))
            .background(NewTheme.Card)
            .clickable(enabled = !busy, onClick = onOpen)
            .padding(start = 14.dp, end = 4.dp, top = 10.dp, bottom = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            imageVector = TablerIcons.Outline.PlugConnected,
            contentDescription = null,
            tint = if (row.tunneled) NewTheme.Success else NewTheme.TextMuted,
            modifier = Modifier.size(18.dp),
        )
        Column(modifier = Modifier.weight(1f).padding(start = 12.dp)) {
            Text(
                text = row.port.toString(),
                color = NewTheme.TextPrimary,
                fontSize = 16.sp,
                fontWeight = FontWeight.SemiBold,
                fontFamily = ConsoleMonoFamily,
            )
            Text(
                text = projectName ?: row.projectId ?: "Not tied to a project",
                color = NewTheme.TextMuted,
                fontSize = 12.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
        if (busy) {
            CircularProgressIndicator(modifier = Modifier.size(14.dp), strokeWidth = 2.dp, color = NewTheme.TextSecondary)
        } else {
            TintPill(
                text = if (row.tunneled) "Tunneled" else "LAN only",
                tint = if (row.tunneled) NewTheme.Success else NewTheme.TextSecondary,
            )
        }
        Box {
            IconButton(onClick = { menuOpen = true }) {
                Icon(
                    imageVector = TablerIcons.Outline.DotsVertical,
                    contentDescription = "Port ${row.port} options",
                    tint = NewTheme.TextMuted,
                    modifier = Modifier.size(18.dp),
                )
            }
            OverflowMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                OverflowMenuItem(
                    label = "Open in browser",
                    icon = TablerIcons.Outline.Globe,
                    onClick = {
                        menuOpen = false
                        onOpen()
                    },
                )
                OverflowMenuItem(
                    label = "Stop forwarding",
                    icon = TablerIcons.Outline.PlugOff,
                    destructive = true,
                    onClick = {
                        menuOpen = false
                        onUnforward()
                    },
                )
            }
        }
    }
}
