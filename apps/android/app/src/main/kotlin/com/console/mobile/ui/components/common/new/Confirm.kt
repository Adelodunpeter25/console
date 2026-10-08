package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.ConfirmDialogBus
import com.console.mobile.ui.theme.NewTheme

/**
 * New-look confirm dialog. It reads the same [ConfirmDialogBus] as the original
 * host, so `confirmAlert(...)` and [ConfirmButton] work unchanged — only the
 * rendering differs. Host it once near the NavHost root *instead of* the old host.
 */
@Composable
fun ConfirmPromptHost() {
    val options by ConfirmDialogBus.state.collectAsState()
    val opts = options ?: return
    ConfirmPrompt(
        title = opts.title,
        message = opts.message,
        buttons = opts.buttons,
        onDismiss = { ConfirmDialogBus.hide() },
        // A tap hides the dialog first, then runs the button's action.
        onPress = { btn -> ConfirmDialogBus.hide(); btn.onPress?.invoke() },
    )
}

/** Inline (non-global) confirm dialog for direct use in a screen. */
@Composable
fun ConfirmPrompt(
    title: String,
    message: String?,
    buttons: List<ConfirmButton>,
    onDismiss: () -> Unit,
    onPress: (ConfirmButton) -> Unit = { btn -> onDismiss(); btn.onPress?.invoke() },
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = NewTheme.Card,
        shape = RoundedCornerShape(NewTheme.CardRadius),
        title = {
            Text(title, color = NewTheme.TextPrimary, fontSize = 18.sp, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth())
        },
        text = if (message != null) {
            { Text(message, color = NewTheme.TextSecondary, fontSize = 15.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) }
        } else null,
        confirmButton = {},
        dismissButton = {
            Column(modifier = Modifier.fillMaxWidth().padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (buttons.size == 2) {
                    // The conventional pair (Cancel / action) shares one row.
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.fillMaxWidth()) {
                        buttons.forEach { PromptButton(it, onPress, Modifier.weight(1f)) }
                    }
                } else {
                    buttons.forEach { PromptButton(it, onPress, Modifier.fillMaxWidth()) }
                }
            }
        },
    )
}

@Composable
private fun PromptButton(btn: ConfirmButton, onPress: (ConfirmButton) -> Unit, modifier: Modifier) {
    ActionButton(
        text = btn.text,
        onClick = { onPress(btn) },
        modifier = modifier,
        kind = when {
            btn.destructive -> ActionButtonKind.Danger
            btn.cancel -> ActionButtonKind.Secondary
            else -> ActionButtonKind.Primary
        },
        // The dialog is Card-coloured, so a Card-coloured button would disappear into it.
        surface = NewTheme.Raised,
    )
}
