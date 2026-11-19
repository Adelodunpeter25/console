package com.console.mobile.feature.chat.html

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.model.HtmlArtifact
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Code
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.Globe

/**
 * Transcript card for a `render` tool result. Renders the page live in a
 * sandboxed WebView; the menu opens it full screen or in the code editor.
 */
@Composable
fun HtmlResultCard(
    artifact: HtmlArtifact,
    onOpenPreview: (HtmlArtifact) -> Unit,
    onOpenEditor: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val displayTitle = artifact.title.ifBlank { artifact.path.substringAfterLast('/') }
    var menuOpen by remember { mutableStateOf(false) }
    val frameHeight = if (artifact.height > 0) artifact.height.coerceIn(160, 600) else 420

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp),
        color = NewTheme.Card,
        shape = RoundedCornerShape(NewTheme.CardRadius),
        border = BorderStroke(1.dp, NewTheme.Divider),
    ) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Icon(
                    TablerIcons.Outline.Globe,
                    contentDescription = null,
                    tint = NewTheme.Accent,
                    modifier = Modifier.size(18.dp),
                )
                Text(
                    displayTitle,
                    color = NewTheme.TextPrimary,
                    fontSize = 14.sp,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.weight(1f),
                )
                Box {
                    IconButton(onClick = { menuOpen = true }, modifier = Modifier.size(32.dp)) {
                        Icon(
                            TablerIcons.Outline.DotsVertical,
                            contentDescription = "Page options",
                            tint = NewTheme.TextSecondary,
                            modifier = Modifier.size(18.dp),
                        )
                    }
                    OverflowMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        OverflowMenuItem(
                            label = "Open in code editor",
                            icon = TablerIcons.Outline.Code,
                            onClick = {
                                menuOpen = false
                                onOpenEditor(artifact.path)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open full screen",
                            icon = TablerIcons.Outline.Globe,
                            onClick = {
                                menuOpen = false
                                onOpenPreview(artifact)
                            },
                        )
                    }
                }
            }

            HtmlPreviewWebView(
                path = artifact.path,
                nestedInList = true,
                modifier = Modifier
                    .fillMaxWidth()
                    .height(frameHeight.dp)
                    .clip(RoundedCornerShape(8.dp)),
            )
        }
    }
}