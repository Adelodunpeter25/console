package com.console.mobile.ui.components

import androidx.compose.foundation.BorderStroke
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
import com.console.mobile.ui.theme.ConsoleColors

/**
 * Shared dropdown / overflow menu. A Material 3 [DropdownMenu] restyled to the
 * app's look: rounded corners, elevated dark surface and a hairline border.
 * Anchor it inside the same Box as the trigger so it drops down from it.
 */
@Composable
fun ConsoleDropdownMenu(
    expanded: Boolean,
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable ColumnScope.() -> Unit,
) {
    DropdownMenu(
        expanded = expanded,
        onDismissRequest = onDismissRequest,
        modifier = modifier,
        shape = RoundedCornerShape(14.dp),
        containerColor = ConsoleColors.SurfaceElevated,
        tonalElevation = 0.dp,
        shadowElevation = 8.dp,
        border = BorderStroke(1.dp, ConsoleColors.Border),
        content = content,
    )
}

/** A single row for [ConsoleDropdownMenu]: optional leading icon + label. */
@Composable
fun ConsoleDropdownMenuItem(
    label: String,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: ImageVector? = null,
    destructive: Boolean = false,
) {
    val tint = if (destructive) ConsoleColors.Destructive else ConsoleColors.TextPrimary
    DropdownMenuItem(
        text = { Text(label, color = tint, fontSize = 14.sp, fontWeight = FontWeight.Medium) },
        onClick = onClick,
        modifier = modifier,
        leadingIcon = icon?.let { { Icon(it, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp)) } },
        contentPadding = PaddingValues(horizontal = 16.dp, vertical = 10.dp),
        colors = MenuDefaults.itemColors(),
    )
}
