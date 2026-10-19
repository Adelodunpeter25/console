package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronDown
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronUp
import io.github.lyxnx.compose.ui.tablericons.outline.Sparkles

/** Collapsible thinking block shown above an assistant reply. */
@Composable
fun ThinkingBlock(text: String, isStreaming: Boolean) {
    var expanded by remember { mutableStateOf(false) }
    Column(modifier = Modifier.padding(bottom = 8.dp)) {
        Row(
            modifier = Modifier.clip(RoundedCornerShape(NewTheme.ChipRadius)).background(NewTheme.Card).clickable { expanded = !expanded }.padding(horizontal = 10.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(TablerIcons.Outline.Sparkles, contentDescription = null, tint = NewTheme.Accent, modifier = Modifier.size(14.dp))
            Text(if (isStreaming) "Thinking…" else "Thought", color = NewTheme.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium, modifier = Modifier.padding(start = 6.dp))
            Icon(if (expanded) TablerIcons.Outline.ChevronUp else TablerIcons.Outline.ChevronDown, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(12.dp))
        }
        if (expanded) {
            Box(modifier = Modifier.fillMaxWidth().padding(top = 6.dp).clip(RoundedCornerShape(NewTheme.FieldRadius)).background(NewTheme.Card).padding(horizontal = 14.dp, vertical = 12.dp)) {
                Text(text, color = NewTheme.TextSecondary, fontSize = 13.sp, fontFamily = ConsoleMonoFamily, lineHeight = 20.sp)
            }
        }
    }
}
