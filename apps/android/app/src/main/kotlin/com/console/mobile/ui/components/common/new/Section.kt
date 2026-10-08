package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.ChevronRight

/**
 * The grouped-card layout of the new look: an accent heading over one rounded
 * card that holds a group of rows.
 *
 * A [Section]'s rows are plain composables laid out in a column; put a
 * [SectionDivider] between them. The card clips its content, so a pressed first
 * or last row follows the rounded corners.
 */
@Composable
fun Section(
    title: String,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Text(
        title, color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium,
        modifier = modifier.padding(start = 12.dp, top = 22.dp, bottom = 8.dp),
    )
    SectionCard(content = content)
}

/**
 * The rounded card on its own, for places where a heading would repeat what is
 * already on screen (a sheet's title, a page header).
 */
@Composable
fun SectionCard(
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier = modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card),
        content = content,
    )
}

/** A hairline between rows inside a [Section], inset to line up with the row text. */
@Composable
fun SectionDivider(startInset: Dp = 18.dp) {
    Box(modifier = Modifier.fillMaxWidth().padding(start = startInset, end = 18.dp).height(1.dp).background(NewTheme.Divider))
}

/** Quiet text for an empty/unavailable state inside a [Section]. */
@Composable
fun Note(text: String, color: Color = NewTheme.TextSecondary) {
    Text(text, color = color, fontSize = 13.sp, modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp))
}

/** A tappable row with a leading icon and a title, for actions such as "Disconnect". */
@Composable
fun ActionRow(
    icon: ImageVector,
    title: String,
    tint: Color = NewTheme.TextPrimary,
    onClick: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 18.dp, vertical = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(22.dp))
        Text(title, color = tint, fontSize = 16.sp, modifier = Modifier.padding(start = 16.dp))
    }
}

/** A row that opens another screen: icon, title, a one-line summary and a chevron. */
@Composable
fun NavRow(icon: ImageVector, title: String, summary: String, onClick: () -> Unit) {
    Row(
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, contentDescription = null, tint = NewTheme.TextPrimary, modifier = Modifier.size(24.dp))
        Column(modifier = Modifier.weight(1f).padding(start = 16.dp)) {
            Text(title, color = NewTheme.TextPrimary, fontSize = 17.sp)
            Text(summary, color = NewTheme.TextMuted, fontSize = 13.sp, modifier = Modifier.padding(top = 1.dp))
        }
        Icon(TablerIcons.Outline.ChevronRight, contentDescription = null, tint = NewTheme.TextMuted, modifier = Modifier.size(20.dp))
    }
}
