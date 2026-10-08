package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronLeft
import io.github.lyxnx.compose.ui.tablericons.outline.Settings

/** Standard header row height: a 40dp control plus 10dp above and below. */
val PageHeaderHeight = 60.dp

/**
 * Screen header: optional back button, a left-aligned title with an optional
 * subtitle, then the screen's [actions] and an optional settings cog. Every screen
 * uses the same left alignment, so there is no option to centre the title.
 *
 * The row has a fixed height so the back button and title sit at the same spot
 * on every screen — sizing from content made headers with a subtitle taller,
 * which pushed their back button lower. The title column takes the leftover
 * space, so a long title ellipsizes before it can reach the actions. Safe-area
 * is handled by the Scaffold / WindowInsets, so there is no manual top padding.
 */
@Composable
fun PageHeader(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    onBack: (() -> Unit)? = null,
    showSettings: Boolean = false,
    onSettingsPress: (() -> Unit)? = null,
    actions: (@Composable RowScope.() -> Unit)? = null,
) {
    Row(
        modifier = modifier.fillMaxWidth().height(PageHeaderHeight).padding(horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (onBack != null) {
            CircleIconButton(TablerIcons.Outline.ChevronLeft, "Back", onBack, Modifier.padding(end = 12.dp))
        }
        Column(
            modifier = Modifier.weight(1f),
            horizontalAlignment = Alignment.Start,
        ) {
            Text(
                text = title,
                color = NewTheme.TextPrimary,
                fontSize = 22.sp,
                fontWeight = FontWeight.Bold,
                textAlign = TextAlign.Start,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
            if (subtitle != null) {
                Text(
                    text = subtitle,
                    color = NewTheme.TextSecondary,
                    fontSize = 12.sp,
                    textAlign = TextAlign.Start,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
        if (actions != null) {
            Row(verticalAlignment = Alignment.CenterVertically, content = actions)
        }
        if (showSettings) {
            // 12dp to match the back button's trailing gap, so the cog doesn't
            // butt against a preceding header action.
            CircleIconButton(TablerIcons.Outline.Settings, "Settings", { onSettingsPress?.invoke() }, Modifier.padding(start = 12.dp))
        }
    }
}
