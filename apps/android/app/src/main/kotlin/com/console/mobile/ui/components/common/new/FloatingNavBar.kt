package com.console.mobile.ui.components.common.new

import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.GitPullRequest
import io.github.lyxnx.compose.ui.tablericons.outline.Message
import io.github.lyxnx.compose.ui.tablericons.outline.Plus
import io.github.lyxnx.compose.ui.tablericons.outline.Settings
import io.github.lyxnx.compose.ui.tablericons.outline.TrendingUp

enum class FloatingNavTab { Chats, PullRequests, Usage, Settings }

/**
 * Floating pill navigation bar paired with a New Chat FAB (option #3).
 * Shown on top-level destinations (Home, Usage, Settings) and hidden inside
 * active workspaces (Chat, Files, Terminal) so they keep full vertical space.
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun FloatingNavBar(
    activeTab: FloatingNavTab,
    onSelectTab: (FloatingNavTab) -> Unit,
    onNewChat: () -> Unit,
    modifier: Modifier = Modifier,
) {
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(
            modifier = Modifier
                .weight(1f)
                .height(54.dp)
                .shadow(12.dp, CircleShape, ambientColor = Color.Black, spotColor = Color.Black)
                .clip(CircleShape)
                .background(NewTheme.Card)
                .border(1.dp, NewTheme.CardBorder, CircleShape)
                .padding(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            NavPillItem(
                icon = TablerIcons.Outline.Message,
                contentDescription = "Chats",
                selected = activeTab == FloatingNavTab.Chats,
                onClick = { onSelectTab(FloatingNavTab.Chats) },
                modifier = Modifier.weight(1f),
            )
            NavPillItem(
                icon = TablerIcons.Outline.GitPullRequest,
                contentDescription = "Pull Requests",
                selected = activeTab == FloatingNavTab.PullRequests,
                onClick = { onSelectTab(FloatingNavTab.PullRequests) },
                modifier = Modifier.weight(1f),
            )
            NavPillItem(
                icon = TablerIcons.Outline.TrendingUp,
                contentDescription = "Usage",
                selected = activeTab == FloatingNavTab.Usage,
                onClick = { onSelectTab(FloatingNavTab.Usage) },
                modifier = Modifier.weight(1f),
            )
            NavPillItem(
                icon = TablerIcons.Outline.Settings,
                contentDescription = "Settings",
                selected = activeTab == FloatingNavTab.Settings,
                onClick = { onSelectTab(FloatingNavTab.Settings) },
                modifier = Modifier.weight(1f),
            )
        }

        val fabInteraction = remember { MutableInteractionSource() }
        val fabPressed by fabInteraction.collectIsPressedAsState()
        val fabScale by animateFloatAsState(
            targetValue = if (fabPressed) 0.92f else 1f,
            animationSpec = MaterialTheme.motionScheme.fastSpatialSpec(),
            label = "newChatFabScale",
        )
        val fabShape = RoundedCornerShape(18.dp)
        Box(
            modifier = Modifier
                .size(54.dp)
                .graphicsLayer {
                    scaleX = fabScale
                    scaleY = fabScale
                }
                .shadow(12.dp, fabShape, ambientColor = Color.Black, spotColor = Color.Black)
                .clip(fabShape)
                .background(NewTheme.Accent)
                .clickable(
                    interactionSource = fabInteraction,
                    indication = null,
                    onClickLabel = "New chat",
                    onClick = onNewChat,
                ),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                imageVector = TablerIcons.Outline.Plus,
                contentDescription = "New chat",
                tint = NewTheme.OnPrimary,
                modifier = Modifier.size(22.dp),
            )
        }
    }
}

@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
private fun NavPillItem(
    icon: ImageVector,
    contentDescription: String,
    selected: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val interactionSource = remember { MutableInteractionSource() }
    val pressed by interactionSource.collectIsPressedAsState()
    val scale by animateFloatAsState(
        targetValue = if (pressed) 0.92f else 1f,
        animationSpec = MaterialTheme.motionScheme.fastSpatialSpec(),
        label = "navPillScale",
    )
    val bg by animateColorAsState(
        targetValue = if (selected) NewTheme.AccentWash else Color.Transparent,
        animationSpec = MaterialTheme.motionScheme.fastEffectsSpec(),
        label = "navPillBg",
    )
    val tint by animateColorAsState(
        targetValue = if (selected) NewTheme.Accent else NewTheme.TextSecondary,
        animationSpec = MaterialTheme.motionScheme.fastEffectsSpec(),
        label = "navPillTint",
    )
    Box(
        modifier = modifier
            .height(46.dp)
            .graphicsLayer {
                scaleX = scale
                scaleY = scale
            }
            .clip(CircleShape)
            .background(bg)
            .clickable(
                interactionSource = interactionSource,
                indication = null,
                onClickLabel = contentDescription,
                onClick = onClick,
            ),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            imageVector = icon,
            contentDescription = contentDescription,
            tint = tint,
            modifier = Modifier.size(20.dp),
        )
    }
}
