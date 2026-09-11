package com.console.mobile.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class ConfirmButton(
    val text: String,
    val destructive: Boolean = false,
    val cancel: Boolean = false,
    val onPress: (() -> Unit)? = null,
)

data class ConfirmOptions(
    val title: String,
    val message: String? = null,
    val buttons: List<ConfirmButton> = listOf(ConfirmButton("OK")),
)

/** Global dialog bus — port of confirmDialog$ legend observable. */
object ConfirmDialogBus {
    private val _state = MutableStateFlow<ConfirmOptions?>(null)
    val state: StateFlow<ConfirmOptions?> = _state.asStateFlow()
    fun show(title: String, message: String? = null, buttons: List<ConfirmButton> = listOf(ConfirmButton("OK"))) {
        _state.value = ConfirmOptions(title, message, buttons)
    }
    fun hide() { _state.value = null }
}

fun confirmAlert(title: String, message: String? = null, buttons: List<ConfirmButton> = listOf(ConfirmButton("OK"))) =
    ConfirmDialogBus.show(title, message, buttons)

/**
 * Host once near the NavHost root. Renders AlertDialog when bus has options.
 */
@Composable
fun ConfirmDialogHost() {
    val options by ConfirmDialogBus.state.collectAsState()
    val opts = options ?: return
    AlertDialog(
        onDismissRequest = { ConfirmDialogBus.hide() },
        containerColor = ConsoleColors.Surface,
        shape = RoundedCornerShape(20.dp),
        title = {
            Text(
                opts.title,
                color = ConsoleColors.TextPrimary,
                fontSize = 17.sp,
                fontWeight = FontWeight.SemiBold,
                textAlign = TextAlign.Center,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        text = if (opts.message != null) {
            { Text(opts.message, color = ConsoleColors.TextSecondary, fontSize = 14.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) }
        } else null,
        confirmButton = {},
        dismissButton = {
            Column(modifier = Modifier.fillMaxWidth().padding(top = 8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (opts.buttons.size == 2) {
                    Row(horizontalArrangement = Arrangement.spacedBy(10.dp), modifier = Modifier.fillMaxWidth()) {
                        opts.buttons.forEach { btn -> DialogBtn(btn, Modifier.weight(1f)) }
                    }
                } else {
                    opts.buttons.forEach { btn -> DialogBtn(btn, Modifier.fillMaxWidth()) }
                }
            }
        },
    )
}

@Composable
private fun DialogBtn(btn: ConfirmButton, modifier: Modifier = Modifier) {
    val dismissThenRun: () -> Unit = {
        ConfirmDialogBus.hide()
        btn.onPress?.invoke()
    }
    when {
        btn.destructive -> Button(
            onClick = dismissThenRun,
            modifier = modifier,
            colors = ButtonDefaults.buttonColors(containerColor = ConsoleColors.Destructive.copy(alpha = 0.15f), contentColor = ConsoleColors.Destructive),
        ) { Text(btn.text, fontWeight = FontWeight.SemiBold) }
        btn.cancel -> OutlinedButton(onClick = dismissThenRun, modifier = modifier) { Text(btn.text, color = ConsoleColors.TextSecondary) }
        else -> Button(
            onClick = dismissThenRun,
            modifier = modifier,
            colors = ButtonDefaults.buttonColors(containerColor = Color.White, contentColor = Color.Black),
        ) { Text(btn.text, fontWeight = FontWeight.SemiBold) }
    }
}

/** Inline (non-global) confirm dialog for direct use in screens. */
@Composable
fun ConfirmDialog(
    title: String,
    message: String?,
    buttons: List<ConfirmButton>,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        containerColor = ConsoleColors.Surface,
        shape = RoundedCornerShape(20.dp),
        title = { Text(title, color = ConsoleColors.TextPrimary, fontSize = 17.sp, fontWeight = FontWeight.SemiBold, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) },
        text = if (message != null) {
            { Text(message, color = ConsoleColors.TextSecondary, fontSize = 14.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth()) }
        } else null,
        confirmButton = {},
        dismissButton = {
            Column(modifier = Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                buttons.forEach { btn ->
                    TextButton(onClick = { onDismiss(); btn.onPress?.invoke() }, modifier = Modifier.fillMaxWidth()) {
                        Text(btn.text, color = if (btn.destructive) ConsoleColors.Destructive else ConsoleColors.TextPrimary)
                    }
                }
            }
        },
    )
}
