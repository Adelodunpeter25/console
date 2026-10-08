package com.console.mobile.ui.components

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

/**
 * Global dialog bus. Screens call [confirmAlert]; the host that renders it is
 * `ConfirmPromptHost` in ui/components/common/new.
 */
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
