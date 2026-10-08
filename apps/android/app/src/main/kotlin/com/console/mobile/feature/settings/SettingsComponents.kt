package com.console.mobile.feature.settings

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
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme

/**
 * Shared building blocks for the new settings look: an accent-coloured heading
 * over one rounded card that holds a group of rows.
 */

/** A heading over a single rounded card. [content] lays its rows out in a column. */
@Composable
internal fun SettingsCategory(
    title: String,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    Text(
        title, color = NewTheme.Accent, fontSize = 15.sp, fontWeight = FontWeight.Medium,
        modifier = modifier.padding(start = 12.dp, top = 22.dp, bottom = 8.dp),
    )
    // The card clips its rows, so a pressed first/last row follows the rounded corners.
    Column(
        modifier = Modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.CardRadius)).background(NewTheme.Card),
        content = content,
    )
}

/** A hairline between rows inside a card, inset to line up with the row text. */
@Composable
internal fun SettingsDivider(startInset: androidx.compose.ui.unit.Dp = 18.dp) {
    Box(modifier = Modifier.fillMaxWidth().padding(start = startInset, end = 18.dp).height(1.dp).background(NewTheme.Divider))
}

/** Quiet text for an empty/loading/unavailable state inside a card. */
@Composable
internal fun SettingsNote(text: String, color: Color = NewTheme.TextSecondary) {
    Text(text, color = color, fontSize = 13.sp, modifier = Modifier.fillMaxWidth().padding(horizontal = 18.dp, vertical = 16.dp))
}

/** A tappable row with a leading icon and a title; used for actions like "Disconnect". */
@Composable
internal fun SettingsActionRow(
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
