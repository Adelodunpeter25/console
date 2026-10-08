package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Plus

enum class ActionButtonKind { Primary, Secondary, Danger }

/**
 * Filled, rounded button. [ActionButtonKind.Secondary] is a raised card-coloured
 * surface (no outline), [ActionButtonKind.Danger] is red text on the same
 * surface, [ActionButtonKind.Primary] is the white call to action.
 *
 * [compact] is the small inline size for buttons that sit inside a row.
 */
@Composable
fun ActionButton(
    text: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    kind: ActionButtonKind = ActionButtonKind.Secondary,
    enabled: Boolean = true,
    loading: Boolean = false,
    icon: ImageVector? = null,
    compact: Boolean = false,
) {
    val fill = when {
        kind == ActionButtonKind.Primary -> if (enabled) NewTheme.Primary else NewTheme.PrimaryDisabled
        else -> NewTheme.Card
    }
    val content = when {
        kind == ActionButtonKind.Primary -> if (enabled) NewTheme.OnPrimary else NewTheme.TextMuted
        !enabled -> NewTheme.TextMuted
        kind == ActionButtonKind.Danger -> NewTheme.Danger
        else -> NewTheme.TextPrimary
    }
    Row(
        modifier = modifier.clip(RoundedCornerShape(if (compact) NewTheme.ChipRadius else NewTheme.FieldRadius)).background(fill)
            .clickable(enabled = enabled && !loading, onClick = onClick)
            .padding(horizontal = if (compact) 14.dp else 20.dp, vertical = if (compact) 9.dp else 15.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.Center,
    ) {
        when {
            loading -> CircularProgressIndicator(color = content, strokeWidth = 2.dp, modifier = Modifier.size(if (compact) 13.dp else 16.dp))
            icon != null -> Icon(icon, contentDescription = null, tint = content, modifier = Modifier.size(if (compact) 14.dp else 18.dp))
        }
        if (loading || icon != null) Spacer(Modifier.size(8.dp))
        Text(text, color = content, fontSize = if (compact) 13.sp else 16.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
    }
}

/** Round "+" for a screen header, to add an item (environment, server, folder). */
@Composable
fun AddButton(contentDescription: String, onClick: () -> Unit) {
    IconButton(onClick = onClick, modifier = Modifier.size(40.dp)) {
        Box(
            modifier = Modifier.size(40.dp).clip(CircleShape).background(NewTheme.Card),
            contentAlignment = Alignment.Center,
        ) {
            Icon(TablerIcons.Outline.Plus, contentDescription = contentDescription, tint = NewTheme.TextPrimary)
        }
    }
}
