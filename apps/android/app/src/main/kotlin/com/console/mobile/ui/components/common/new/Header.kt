package com.console.mobile.ui.components.common.new

import android.os.Build
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.blur
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ArrowLeft
import io.github.lyxnx.compose.ui.tablericons.outline.Settings

/** Standard header row height: a 40dp control plus 10dp above and below. */
val PageHeaderHeight = 60.dp

/**
 * Screen header: optional back button, a left-aligned title with an optional
 * subtitle, then the screen's [actions] and an optional settings cog.
 *
 * Supports [blurred] to render with a frosted/progressive blur background scrim.
 */
@Composable
fun PageHeader(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    onBack: (() -> Unit)? = null,
    showSettings: Boolean = false,
    onSettingsPress: (() -> Unit)? = null,
    blurred: Boolean = false,
    backgroundColor: Color = NewTheme.Background,
    blurRadius: Dp = 20.dp,
    actions: (@Composable RowScope.() -> Unit)? = null,
) {
    Box(
        modifier = modifier.fillMaxWidth().height(PageHeaderHeight),
    ) {
        if (blurred) {
            Box(
                modifier = Modifier
                    .matchParentSize()
                    .then(
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                            Modifier.blur(blurRadius)
                        } else {
                            Modifier
                        }
                    )
                    .background(
                        Brush.verticalGradient(
                            0.0f to backgroundColor.copy(alpha = 0.95f),
                            0.75f to backgroundColor.copy(alpha = 0.85f),
                            1.0f to backgroundColor.copy(alpha = 0.65f),
                        )
                    ),
            )
        }
        Row(
            modifier = Modifier.fillMaxWidth().height(PageHeaderHeight).padding(horizontal = 16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            if (onBack != null) {
                IconButton(
                    onClick = onBack,
                    modifier = Modifier.padding(end = 12.dp).size(36.dp),
                ) {
                    Icon(
                        imageVector = TablerIcons.Outline.ArrowLeft,
                        contentDescription = "Back",
                        tint = NewTheme.TextPrimary,
                        modifier = Modifier.size(22.dp),
                    )
                }
            }
            Column(
                modifier = Modifier.weight(1f),
                horizontalAlignment = Alignment.Start,
            ) {
                Text(
                    text = title,
                    color = NewTheme.TextPrimary,
                    fontSize = 20.sp,
                    fontWeight = FontWeight.SemiBold,
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
                HeaderIconButton(TablerIcons.Outline.Settings, "Settings", { onSettingsPress?.invoke() }, Modifier.padding(start = 12.dp))
            }
        }
    }
}
