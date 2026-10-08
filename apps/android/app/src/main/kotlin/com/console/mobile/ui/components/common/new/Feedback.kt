package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle

/** Centered spinner with an optional caption, for a screen that is still loading. */
@Composable
fun LoadingState(caption: String? = null, modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            CircularProgressIndicator(color = NewTheme.TextPrimary, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
            if (caption != null) Text(caption, color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(top = 12.dp))
        }
    }
}

/** A tinted notice (errors by default) that sits in the page, outside any [Section]. */
@Composable
fun Banner(text: String, tint: Color = NewTheme.Danger, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.FieldRadius)).background(tint.copy(alpha = 0.12f)).padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
        Text(text, color = tint, fontSize = 13.sp, modifier = Modifier.padding(start = 10.dp))
    }
}
