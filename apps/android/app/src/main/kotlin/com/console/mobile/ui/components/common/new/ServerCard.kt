package com.console.mobile.ui.components.common.new

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

/**
 * The server card: where the server is, and the code that lets this device
 * join it.
 *
 * The two fields are what the server prints (`console pair`): a user copies
 * one line and types two short things.
 *
 * There is no divider between the fields. The card is already a group, the
 * labels say which field is which, and a rule across the middle reads as a
 * stray line rather than as structure.
 */
@Composable
fun ServerCard(
    host: String,
    code: String,
    onHostChange: (String) -> Unit,
    onCodeChange: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    SectionCard(modifier = modifier) {
        TextFieldRow(
            "Host",
            host,
            onHostChange,
            "192.168.1.100:8080",
            monospace = true,
        )
        TextFieldRow(
            "Pairing code",
            code,
            onCodeChange,
            "abc-123-xyz",
            monospace = true,
            // The card clips its content to a rounded corner, so the last
            // field has to stand off the bottom edge or its own rounded
            // corners get cut off.
            last = true,
        )
    }
}
