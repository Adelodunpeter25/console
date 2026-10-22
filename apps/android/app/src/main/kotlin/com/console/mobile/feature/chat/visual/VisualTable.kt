package com.console.mobile.feature.chat.visual

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme

/** Compact summary table; scrolls horizontally inside its own card, never the transcript. */
@Composable
fun VisualTable(t: VisualBlock.Table) {
    val numeric = remember(t) {
        t.columns.indices.map { c -> t.rows.isNotEmpty() && t.rows.all { r -> r.getOrNull(c)?.let(::looksNumeric) == true } }
    }
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        t.caption?.let { Text(it, color = NewTheme.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium) }
        Column(Modifier.horizontalScroll(rememberScrollState())) {
            Row {
                t.columns.forEachIndexed { c, name ->
                    Cell(name, numeric[c], header = true)
                }
            }
            HorizontalDivider(color = NewTheme.Divider)
            t.rows.forEach { row ->
                // One spoken sentence per row so TalkBack reads header + value pairs.
                val spoken = row.mapIndexed { c, v -> "${t.columns.getOrElse(c) { "" }}: $v" }.joinToString(", ")
                Row(Modifier.semantics(mergeDescendants = true) { contentDescription = spoken }) {
                    row.forEachIndexed { c, cell -> Cell(cell, numeric.getOrElse(c) { false }, header = false) }
                }
            }
        }
    }
}

@Composable
private fun Cell(text: String, rightAlign: Boolean, header: Boolean) {
    Text(
        text,
        color = if (header) NewTheme.TextSecondary else NewTheme.TextPrimary,
        fontSize = 12.sp,
        fontWeight = if (header) FontWeight.Medium else FontWeight.Normal,
        textAlign = if (rightAlign) TextAlign.End else TextAlign.Start,
        modifier = Modifier.widthIn(min = 84.dp).padding(horizontal = 8.dp, vertical = 6.dp),
    )
}
