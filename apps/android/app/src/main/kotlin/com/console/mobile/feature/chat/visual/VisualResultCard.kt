package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.data.model.VisualArtifact
import com.console.mobile.ui.theme.NewTheme

/**
 * Renders workspace image visuals (from read_file). Legacy chart/metric/table
 * specs show a compact fallback.
 */
@Composable
fun VisualResultCard(artifact: VisualArtifact, modifier: Modifier = Modifier) {
    val parsed = remember(artifact) { parseVisual(artifact) }
    when (parsed) {
        is VisualParse.Ok -> {
            Surface(
                modifier = modifier.fillMaxWidth().padding(vertical = 6.dp),
                color = NewTheme.Background,
                shape = RoundedCornerShape(12.dp),
            ) {
                Column(
                    modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    parsed.spec.blocks.forEach { block ->
                        when (block) {
                            is VisualBlock.Image -> WorkspaceImage(path = block.path, alt = block.alt)
                        }
                    }
                }
            }
        }
        VisualParse.Unsupported, VisualParse.Invalid -> {
            Surface(
                modifier = modifier.fillMaxWidth().padding(vertical = 6.dp),
                color = NewTheme.Background,
                shape = RoundedCornerShape(12.dp),
            ) {
                Text(
                    "Visual unavailable",
                    color = NewTheme.TextMuted,
                    fontSize = 13.sp,
                    modifier = Modifier.padding(horizontal = 4.dp, vertical = 8.dp),
                )
            }
        }
    }
}
