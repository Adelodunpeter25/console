package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check

/**
 * The shared bottom sheet of the new look: a full-height modal on the page
 * background, with an optional [title] and a padded content column.
 *
 * [scrollable] wraps the content in a vertical scroll, which suits a handful of
 * rows. Turn it off when the content owns its own scrolling (a `LazyColumn`):
 * nesting two vertical scrollers crashes, so such content should size itself
 * with `Modifier.weight(1f, fill = false)` instead.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun BaseSheet(
    onDismiss: () -> Unit,
    title: String? = null,
    scrollable: Boolean = true,
    containerColor: Color = NewTheme.Background,
    content: @Composable ColumnScope.() -> Unit,
) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        containerColor = containerColor,
    ) {
        Column(
            modifier = Modifier.fillMaxWidth()
                .then(if (scrollable) Modifier.verticalScroll(rememberScrollState()) else Modifier)
                .padding(horizontal = 16.dp).padding(bottom = 40.dp),
        ) {
            if (title != null) SheetTitle(title)
            content()
        }
    }
}

/** Title of a [BaseSheet]. Public so a sheet that supplies its own layout can still match. */
@Composable
fun SheetTitle(title: String) {
    Text(title, color = NewTheme.TextPrimary, fontSize = 20.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.padding(start = 4.dp, bottom = 8.dp))
}

/**
 * A selectable row for a [Section] inside a sheet: title, optional subtitle, an
 * optional [leading] glyph (a status dot), an optional [trailing] control (a
 * star, say), and a check on the selection.
 */
@Composable
fun OptionRow(
    title: String,
    modifier: Modifier = Modifier,
    subtitle: String? = null,
    selected: Boolean = false,
    monoSubtitle: Boolean = false,
    leading: (@Composable () -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
    onClick: () -> Unit,
) {
    Row(
        modifier = modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 18.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            leading()
            Spacer(modifier = Modifier.size(14.dp))
        }
        Column(modifier = Modifier.weight(1f)) {
            Text(title, color = NewTheme.TextPrimary, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (!subtitle.isNullOrBlank()) {
                Text(
                    subtitle, color = NewTheme.TextMuted, fontSize = if (monoSubtitle) 12.sp else 13.sp,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                    fontFamily = if (monoSubtitle) ConsoleMonoFamily else null,
                    modifier = Modifier.padding(top = 1.dp),
                )
            }
        }
        if (trailing != null) {
            trailing()
            Spacer(modifier = Modifier.size(12.dp))
        }
        if (selected) Icon(TablerIcons.Outline.Check, contentDescription = "Selected", tint = NewTheme.Accent, modifier = Modifier.size(20.dp))
    }
}

/**
 * A pill in a horizontal tab row (the provider tabs). The selected one takes the
 * accent; [leading] is an optional glyph before the label.
 */
@Composable
fun TabPill(
    label: String,
    selected: Boolean,
    modifier: Modifier = Modifier,
    leading: (@Composable () -> Unit)? = null,
    onClick: () -> Unit,
) {
    Row(
        modifier = modifier.clip(RoundedCornerShape(NewTheme.ChipRadius))
            .background(if (selected) NewTheme.Accent.copy(alpha = 0.18f) else NewTheme.Card)
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 9.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (leading != null) {
            leading()
            Spacer(Modifier.size(6.dp))
        }
        Text(label, color = if (selected) NewTheme.Accent else NewTheme.TextSecondary, fontSize = 14.sp, fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Medium, maxLines = 1)
    }
}

/** A square tab with just an icon (the favourites tab). */
@Composable
fun IconTabPill(
    icon: androidx.compose.ui.graphics.vector.ImageVector,
    contentDescription: String,
    selected: Boolean,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
) {
    Box(
        modifier = modifier.size(38.dp).clip(RoundedCornerShape(NewTheme.ChipRadius))
            .background(if (selected) NewTheme.Accent.copy(alpha = 0.18f) else NewTheme.Card)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = contentDescription, tint = if (selected) NewTheme.Accent else NewTheme.TextSecondary, modifier = Modifier.size(18.dp))
    }
}
