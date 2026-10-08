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
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.AlertTriangle
import io.github.lyxnx.compose.ui.tablericons.outline.Plus

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

// ---------------------------------------------------------------------------
// Forms, buttons and states shared by the settings screens.
// ---------------------------------------------------------------------------

/** Round "+" in a screen header, for adding an item (environment, MCP server, folder). */
@Composable
internal fun HeaderAddButton(contentDescription: String, onClick: () -> Unit) {
    androidx.compose.material3.IconButton(onClick = onClick, modifier = Modifier.size(40.dp)) {
        Box(
            modifier = Modifier.size(40.dp).clip(androidx.compose.foundation.shape.CircleShape).background(NewTheme.Card),
            contentAlignment = Alignment.Center,
        ) {
            Icon(TablerIcons.Outline.Plus, contentDescription = contentDescription, tint = NewTheme.TextPrimary)
        }
    }
}

/** Centered spinner with an optional caption, for a screen that is still loading. */
@Composable
internal fun SettingsLoading(caption: String? = null, modifier: Modifier = Modifier) {
    Box(modifier = modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) {
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            androidx.compose.material3.CircularProgressIndicator(color = NewTheme.TextPrimary, strokeWidth = 2.dp, modifier = Modifier.size(24.dp))
            if (caption != null) Text(caption, color = NewTheme.TextSecondary, fontSize = 13.sp, modifier = Modifier.padding(top = 12.dp))
        }
    }
}

/** A tinted notice (errors, warnings) inside the page, outside any card. */
@Composable
internal fun SettingsBanner(text: String, tint: Color = NewTheme.Danger, modifier: Modifier = Modifier) {
    Row(
        modifier = modifier.fillMaxWidth().clip(RoundedCornerShape(NewTheme.FieldRadius)).background(tint.copy(alpha = 0.12f)).padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(TablerIcons.Outline.AlertTriangle, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
        Text(text, color = tint, fontSize = 13.sp, modifier = Modifier.padding(start = 10.dp))
    }
}

/** A form field: accent heading over a card holding the text field. */
@Composable
internal fun SettingsField(
    label: String,
    value: String,
    onValueChange: (String) -> Unit,
    placeholder: String,
    singleLine: Boolean = true,
    monospace: Boolean = false,
) {
    SettingsCategory(label) {
        androidx.compose.material3.TextField(
            value = value,
            onValueChange = onValueChange,
            placeholder = { Text(placeholder, color = NewTheme.TextGhost, fontSize = 15.sp) },
            singleLine = singleLine,
            textStyle = androidx.compose.ui.text.TextStyle(
                color = NewTheme.TextPrimary, fontSize = 16.sp,
                fontFamily = if (monospace) com.console.mobile.ui.theme.ConsoleMonoFamily else null,
            ),
            colors = androidx.compose.material3.TextFieldDefaults.colors(
                focusedContainerColor = Color.Transparent,
                unfocusedContainerColor = Color.Transparent,
                disabledContainerColor = Color.Transparent,
                focusedIndicatorColor = Color.Transparent,
                unfocusedIndicatorColor = Color.Transparent,
                disabledIndicatorColor = Color.Transparent,
                cursorColor = NewTheme.Accent,
            ),
            modifier = Modifier.fillMaxWidth(),
        )
    }
}

/** A choice between a few options, as pills in a card: the selected one takes the accent. */
@Composable
internal fun <T> SettingsChoice(
    label: String,
    options: List<Pair<T, String>>,
    selected: T,
    onSelect: (T) -> Unit,
) {
    SettingsCategory(label) {
        Row(modifier = Modifier.fillMaxWidth().padding(10.dp), horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(8.dp)) {
            options.forEach { (value, text) ->
                val on = value == selected
                Box(
                    modifier = Modifier.weight(1f).clip(RoundedCornerShape(NewTheme.ChipRadius))
                        .background(if (on) NewTheme.Accent.copy(alpha = 0.18f) else Color.White.copy(alpha = 0.05f))
                        .clickable { onSelect(value) }.padding(vertical = 12.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(text, color = if (on) NewTheme.Accent else NewTheme.TextSecondary, fontSize = 14.sp, fontWeight = if (on) FontWeight.SemiBold else FontWeight.Normal, maxLines = 1)
                }
            }
        }
    }
}

enum class SettingsButtonKind { Primary, Secondary, Danger }

/**
 * Filled, rounded button. Secondary is a raised card-coloured surface (no outline),
 * Danger is red text on the same surface, Primary is the white call to action.
 */
@Composable
internal fun SettingsButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    kind: SettingsButtonKind = SettingsButtonKind.Secondary,
    enabled: Boolean = true,
    loading: Boolean = false,
    icon: ImageVector? = null,
    compact: Boolean = false,
) {
    val fill = when {
        kind == SettingsButtonKind.Primary -> if (enabled) NewTheme.Primary else NewTheme.PrimaryDisabled
        else -> NewTheme.Card
    }
    val content = when {
        kind == SettingsButtonKind.Primary -> if (enabled) NewTheme.OnPrimary else NewTheme.TextMuted
        !enabled -> NewTheme.TextMuted
        kind == SettingsButtonKind.Danger -> NewTheme.Danger
        else -> NewTheme.TextPrimary
    }
    Row(
        modifier = modifier.clip(RoundedCornerShape(if (compact) NewTheme.ChipRadius else NewTheme.FieldRadius)).background(fill)
            .clickable(enabled = enabled && !loading, onClick = onClick)
            .padding(horizontal = if (compact) 14.dp else 20.dp, vertical = if (compact) 9.dp else 15.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.Center,
    ) {
        when {
            loading -> androidx.compose.material3.CircularProgressIndicator(color = content, strokeWidth = 2.dp, modifier = Modifier.size(if (compact) 13.dp else 16.dp))
            icon != null -> Icon(icon, contentDescription = null, tint = content, modifier = Modifier.size(if (compact) 14.dp else 18.dp))
        }
        if (loading || icon != null) androidx.compose.foundation.layout.Spacer(Modifier.size(8.dp))
        Text(text, color = content, fontSize = if (compact) 13.sp else 16.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
    }
}
