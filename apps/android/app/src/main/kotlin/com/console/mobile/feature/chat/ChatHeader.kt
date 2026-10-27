package com.console.mobile.feature.chat

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.components.common.new.HeaderIconButton
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.components.common.new.PageHeader
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.BrandGit
import io.github.lyxnx.compose.ui.tablericons.outline.DeviceMobile
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2

/** Chat title bar with the shortcuts into the project's other tabs. */
@Composable
internal fun ChatHeader(
    title: String,
    onBack: () -> Unit,
    onOpenFiles: () -> Unit,
    onOpenChanges: () -> Unit,
    onOpenPorts: () -> Unit,
    onOpenDevices: () -> Unit,
    onOpenTerminal: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var overflowMenu by remember { mutableStateOf(false) }
    PageHeader(
        title = title,
        blurred = true,
        modifier = modifier,
        onBack = onBack,
        actions = {
            HeaderIconButton(TablerIcons.Outline.Folder, "Open file explorer", onClick = onOpenFiles)
            Box(modifier = Modifier.padding(start = 4.dp)) {
                HeaderIconButton(TablerIcons.Outline.DotsVertical, "More options", onClick = { overflowMenu = true })
                OverflowMenu(expanded = overflowMenu, onDismissRequest = { overflowMenu = false }) {
                    OverflowMenuItem(
                        label = "Open diff",
                        icon = TablerIcons.Outline.BrandGit,
                        onClick = {
                            overflowMenu = false
                            onOpenChanges()
                        },
                    )
                    OverflowMenuItem(
                        label = "Open ports",
                        icon = TablerIcons.Outline.PlugConnected,
                        onClick = {
                            overflowMenu = false
                            onOpenPorts()
                        },
                    )
                    OverflowMenuItem(
                        label = "Open devices",
                        icon = TablerIcons.Outline.DeviceMobile,
                        onClick = {
                            overflowMenu = false
                            onOpenDevices()
                        },
                    )
                    OverflowMenuItem(
                        label = "Open terminal",
                        icon = TablerIcons.Outline.Terminal2,
                        onClick = {
                            overflowMenu = false
                            onOpenTerminal()
                        },
                    )
                }
            }
        },
    )
}
