package com.console.mobile.feature.chat.html

import android.util.Base64
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
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
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import coil3.request.ImageRequest
import com.console.mobile.data.model.HtmlArtifact
import com.console.mobile.ui.components.common.new.OverflowMenu
import com.console.mobile.ui.components.common.new.OverflowMenuItem
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Code
import io.github.lyxnx.compose.ui.tablericons.outline.DotsVertical
import io.github.lyxnx.compose.ui.tablericons.outline.ExternalLink
import io.github.lyxnx.compose.ui.tablericons.outline.Globe

/**
 * Transcript card for a `render` tool result. Shows the snapshot PNG when
 * available (or a compact interactive-page banner) and opens the sandboxed
 * preview screen, code editor, or external browser.
 */
@Composable
fun HtmlResultCard(
    artifact: HtmlArtifact,
    onOpenPreview: (HtmlArtifact) -> Unit,
    onOpenEditor: (String) -> Unit,
    onOpenExternal: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val displayTitle = artifact.title.ifBlank { artifact.path.substringAfterLast('/') }
    val snapshotBytes = remember(artifact.snapshotBase64) {
        artifact.snapshotBase64?.let { runCatching { Base64.decode(it, Base64.DEFAULT) }.getOrNull() }
    }
    var menuOpen by remember { mutableStateOf(false) }

    Surface(
        modifier = modifier
            .fillMaxWidth()
            .padding(vertical = 6.dp)
            .clickable { onOpenPreview(artifact) },
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
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        displayTitle,
                        color = NewTheme.TextPrimary,
                        fontSize = 14.sp,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    Text(
                        artifact.path.substringAfterLast('/'),
                        color = NewTheme.TextMuted,
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
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
                            label = "Open in browser",
                            icon = TablerIcons.Outline.Globe,
                            onClick = {
                                menuOpen = false
                                onOpenPreview(artifact)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open in code editor",
                            icon = TablerIcons.Outline.Code,
                            onClick = {
                                menuOpen = false
                                onOpenEditor(artifact.path)
                            },
                        )
                        OverflowMenuItem(
                            label = "Open externally",
                            icon = TablerIcons.Outline.ExternalLink,
                            onClick = {
                                menuOpen = false
                                onOpenExternal(artifact.path)
                            },
                        )
                    }
                }
            }

            if (snapshotBytes != null) {
                AsyncImage(
                    model = ImageRequest.Builder(context).data(snapshotBytes).build(),
                    contentDescription = displayTitle,
                    contentScale = ContentScale.FillWidth,
                    modifier = Modifier
                        .fillMaxWidth()
                        .heightIn(max = 260.dp)
                        .clip(RoundedCornerShape(8.dp)),
                )
            }

            Surface(
                color = NewTheme.Raised,
                shape = RoundedCornerShape(8.dp),
                modifier = Modifier.fillMaxWidth(),
            ) {
                Row(
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 9.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.SpaceBetween,
                ) {
                    Text(
                        "Interactive HTML page",
                        color = NewTheme.TextSecondary,
                        fontSize = 12.sp,
                    )
                    Text(
                        "Open preview →",
                        color = NewTheme.Accent,
                        fontSize = 12.sp,
                        fontWeight = FontWeight.Medium,
                    )
                }
            }
        }
    }
}
