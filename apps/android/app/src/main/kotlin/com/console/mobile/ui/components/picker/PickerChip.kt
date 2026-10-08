package com.console.mobile.ui.components.picker

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.icons.getProviderIconKey
import com.console.mobile.ui.components.ProviderIcon
import com.console.mobile.ui.theme.ConsoleColors

/**
 * Compact chip that opens a picker — shows the provider logo instead of the
 * generic glyph when the current value names a known provider.
 */
@Composable
fun PickerChip(
    icon: ImageVector,
    label: String,
    modifier: Modifier = Modifier,
    provider: String? = null,
    enabled: Boolean = true,
    onClick: () -> Unit,
) {
    Row(
        modifier = modifier.alpha(if (enabled) 1f else 0.45f)
            .clip(RoundedCornerShape(8.dp))
            .background(ConsoleColors.CardAlt)
            .border(1.dp, ConsoleColors.BorderSubtle, RoundedCornerShape(8.dp))
            .clickable(enabled = enabled, onClick = onClick)
            .padding(horizontal = 9.dp, vertical = 5.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (provider != null && getProviderIconKey(provider) != null) {
            ProviderIcon(provider = provider, sizeDp = 13)
        } else {
            Icon(icon, contentDescription = null, tint = ConsoleColors.TextSecondary, modifier = Modifier.size(13.dp))
        }
        Text(
            label, color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontWeight = FontWeight.Medium,
            maxLines = 1, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(start = 5.dp),
        )
    }
}
