package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.components.common.new.FloatingGlassIconButton
import com.console.mobile.ui.components.common.new.LocalHazeState
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.components.common.new.liquidGlass
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.BrandGit
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronLeft
import io.github.lyxnx.compose.ui.tablericons.outline.DeviceMobile
import io.github.lyxnx.compose.ui.tablericons.outline.Dots
import io.github.lyxnx.compose.ui.tablericons.outline.Folder
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Terminal2

/**
 * Floating dynamic island chat header with liquid glass styling:
 * - Floating circular glass back button on the left
 * - Floating glass status / persona pill in the center
 * - Floating circular glass options button with dropdown menu on the right
 */
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
    val hazeState = LocalHazeState.current

    Row(
        modifier = modifier
            .fillMaxWidth()
            .statusBarsPadding()
            .padding(horizontal = 16.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        // Floating Back Button
        FloatingGlassIconButton(
            icon = TablerIcons.Outline.ChevronLeft,
            contentDescription = "Back",
            onClick = onBack,
            hazeState = hazeState,
            size = 42.dp,
            iconSize = 22.dp,
        )

        // Floating Dynamic Island Center Pill
        Box(
            modifier = Modifier
                .weight(1f, fill = false)
                .padding(horizontal = 8.dp)
                .liquidGlass(
                    hazeState = hazeState,
                    shape = RoundedCornerShape(22.dp),
                    backgroundColor = Color(0xFF16161E).copy(alpha = 0.72f),
                    borderWidth = 1.dp,
                    blurRadius = 24.dp,
                    elevation = 6.dp,
                )
                .padding(horizontal = 14.dp, vertical = 7.dp),
            contentAlignment = Alignment.Center,
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.Center,
            ) {
                // Purple mascot / Hermes bot icon with eyes
                Box(
                    modifier = Modifier
                        .size(22.dp)
                        .clip(RoundedCornerShape(6.5.dp))
                        .background(Color(0xFF8B5CF6)),
                    contentAlignment = Alignment.Center,
                ) {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(3.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Box(
                            modifier = Modifier
                                .size(3.2.dp)
                                .clip(CircleShape)
                                .background(Color.White),
                        )
                        Box(
                            modifier = Modifier
                                .size(3.2.dp)
                                .clip(CircleShape)
                                .background(Color.White),
                        )
                    }
                }

                Spacer(modifier = Modifier.width(8.dp))

                Text(
                    text = title.ifBlank { "Hermes" },
                    color = NewTheme.TextPrimary,
                    fontSize = 13.5.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }

        // Floating Options Button
        Box(contentAlignment = Alignment.TopEnd) {
            FloatingGlassIconButton(
                icon = TablerIcons.Outline.Dots,
                contentDescription = "More options",
                onClick = { overflowMenu = true },
                hazeState = hazeState,
                size = 42.dp,
                iconSize = 20.dp,
            )
            OverflowMenu(
                expanded = overflowMenu,
                onDismissRequest = { overflowMenu = false },
            ) {
                OverflowMenuItem(
                    label = "Open file explorer",
                    icon = TablerIcons.Outline.Folder,
                    onClick = {
                        overflowMenu = false
                        onOpenFiles()
                    },
                )
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
    }
}
