package com.console.mobile.ui.components.picker

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.heightIn
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.core.icons.getProviderIconKey
import com.console.mobile.ui.components.ProviderIcon
import com.console.mobile.ui.components.common.new.floatingSurface
import com.console.mobile.ui.theme.NewTheme

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
    /** Fill. [NewTheme.Card] on the page; pass [NewTheme.Raised] when the chip sits on a card (the composer bubble). */
    surface: Color = NewTheme.Card,
    shape: androidx.compose.ui.graphics.Shape = RoundedCornerShape(10.dp),
    onClick: () -> Unit,
) {
    Row(
        modifier = modifier
            .alpha(if (enabled) 1f else 0.45f)
            .floatingSurface(
                shape = shape,
                backgroundColor = surface,
                borderColor = Color.White.copy(alpha = 0.08f),
                elevation = 4.dp,
            )
            .clickable(enabled = enabled, onClick = onClick)
            .heightIn(min = 34.dp)
            .padding(horizontal = 12.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (provider != null && getProviderIconKey(provider) != null) {
            ProviderIcon(provider = provider, sizeDp = 14)
        } else {
            Icon(icon, contentDescription = null, tint = NewTheme.TextSecondary, modifier = Modifier.size(14.dp))
        }
        Text(
            label, color = NewTheme.TextSecondary, fontSize = 12.sp, fontWeight = FontWeight.Medium,
            maxLines = 1, overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(start = 5.dp),
        )
    }
}
