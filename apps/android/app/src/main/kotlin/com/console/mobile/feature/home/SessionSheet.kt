package com.console.mobile.feature.home

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.AppContainer
import com.console.mobile.data.model.UpdateSessionDto
import com.console.mobile.ui.components.ConfirmButton
import com.console.mobile.ui.components.confirmAlert
import com.console.mobile.ui.components.common.new.ActionButton
import com.console.mobile.ui.components.common.new.ActionButtonKind
import com.console.mobile.ui.components.common.new.ActionRow
import com.console.mobile.ui.components.common.new.BaseSheet
import com.console.mobile.ui.components.common.new.SectionCard
import com.console.mobile.ui.components.common.new.SectionDivider
import com.console.mobile.ui.components.common.new.TextInput
import com.console.mobile.ui.theme.NewTheme
import console.v1.SessionHeader
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.filled.Pin
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Pin
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** The long-press actions sheet: rename, pin/unpin, delete. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SessionActionsSheet(
    session: SessionHeader,
    onDismiss: () -> Unit,
) {
    var renameOpen by remember(session.id) { mutableStateOf(false) }
    val isPinned = (session.pinned_at ?: 0L) > 0
    BaseSheet(onDismiss = onDismiss) {
        SectionCard {
            ActionRow(icon = TablerIcons.Outline.Edit, title = "Rename") { renameOpen = true }
            SectionDivider()
            ActionRow(
                icon = if (isPinned) TablerIcons.Filled.Pin else TablerIcons.Outline.Pin,
                title = if (isPinned) "Unpin" else "Pin",
            ) {
                val target = session
                onDismiss()
                AppContainer.actionScope.launch {
                    try {
                        withContext(Dispatchers.IO) {
                            AppContainer.projectRepository.updateSession(
                                target.id,
                                UpdateSessionDto(pinned = !isPinned),
                            )
                        }
                    } catch (e: Exception) {
                        confirmAlert("Failed", e.message ?: "Unable to update pin.")
                    }
                }
            }
            SectionDivider()
            ActionRow(icon = TablerIcons.Outline.Trash, title = "Delete", tint = NewTheme.Danger) {
                onDismiss()
                confirmAlert(
                    "Delete Chat",
                    "Are you sure you want to delete \"${session.title}\"?",
                    listOf(
                        ConfirmButton("Cancel", cancel = true),
                        ConfirmButton("Delete", destructive = true, onPress = {
                            AppContainer.actionScope.launch {
                                try {
                                    withContext(Dispatchers.IO) { AppContainer.projectRepository.deleteSession(session.id) }
                                } catch (e: Exception) {
                                    confirmAlert("Failed", e.message ?: "Unable to delete chat.")
                                }
                            }
                        }),
                    ),
                )
            }
        }
    }
    if (renameOpen) {
        var renameValue by remember(session.id) { mutableStateOf(session.title) }
        // A sheet inside the sheet: renaming is a one-field form, so it gets the
        // same field + primary button as every other form instead of a system dialog.
        BaseSheet(onDismiss = { renameOpen = false }, title = "Rename chat") {
            TextInput("Title", renameValue, { renameValue = it }, "Chat title")
            ActionButton(
                text = "Save",
                onClick = {
                    renameOpen = false
                    onDismiss()
                    AppContainer.actionScope.launch {
                        try {
                            withContext(Dispatchers.IO) {
                                AppContainer.projectRepository.updateSession(session.id, UpdateSessionDto(title = renameValue.trim().ifBlank { session.title }))
                            }
                        } catch (e: Exception) {
                            confirmAlert("Failed", e.message ?: "Unable to rename chat.")
                        }
                    }
                },
                kind = ActionButtonKind.Primary,
                enabled = renameValue.isNotBlank(),
                modifier = Modifier.fillMaxWidth().padding(top = 24.dp),
            )
        }
    }
}
