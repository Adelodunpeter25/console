package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MenuDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.NewTheme

/**
 * Overflow / dropdown menu: a Material 3 [DropdownMenu] on the raised surface with
 * the card radius and no border. Anchor it inside the same Box as its trigger so
 * it drops down from it.
 */
@Composable
fun OverflowMenu(
    expanded: Boolean,
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = onDismissRequest,
        modifier = modifier,
        shape = RoundedCornerShape(NewTheme.FieldRadius),
        containerColor = NewTheme.Raised,
        tonalElevation = 0.dp,
        shadowElevation = 8.dp,
        content = content,
    )
}

/** A single row for [OverflowMenu]: optional leading icon and a label. */
@Composable
fun OverflowMenuItem(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    destructive: Boolean = false,
) {
    val tint = if (destructive) NewTheme.Danger else NewTheme.TextPrimary
    DropdownMenuItem(
        text = { Text(label, color = tint, fontSize = 16.sp, fontWeight = FontWeight.Normal) },
        onClick = onClick,
        modifier = modifier,
        leadingIcon = icon?.let { { Icon(it, contentDescription = null, tint = tint, modifier = Modifier.size(20.dp)) } },
        contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
        colors = MenuDefaults.itemColors(),
    )
}
