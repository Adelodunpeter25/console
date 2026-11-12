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
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Edit
import io.github.lyxnx.compose.ui.tablericons.outline.Mail
import io.github.lyxnx.compose.ui.tablericons.outline.MailOpened
import io.github.lyxnx.compose.ui.tablericons.outline.Pin
import io.github.lyxnx.compose.ui.tablericons.outline.Refresh
import io.github.lyxnx.compose.ui.tablericons.outline.Trash
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** The read-state entry a chat's sheet offers. */
internal enum class ReadAction { MarkRead, MarkUnread }

/**
 * Which read-state entry the sheet offers. An unread chat offers "Mark as read".
 * A read chat offers "Mark as unread", except while it is running or once settled:
 * the server refuses both, so the entry is hidden rather than offered and failed.
 */
internal fun readAction(session: SessionHeader): ReadAction? = when {
    session.unread -> ReadAction.MarkRead
    (session.settled_at ?: 0L) <= 0 && session.status != "working" -> ReadAction.MarkUnread
    else -> null
}

/** The long-press actions sheet: mark read/unread, rename, pin/unpin, settle/unsettle, delete. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
internal fun SessionActionsSheet(
    session: SessionHeader,
    onDismiss: () -> Unit,
) {
    var renameOpen by remember(session.id) { mutableStateOf(false) }
    val isPinned = (session.pinned_at ?: 0L) > 0
    val isSettled = (session.settled_at ?: 0L) > 0
    BaseSheet(onDismiss = onDismiss) {
        SectionCard {
            when (readAction(session)) {
                ReadAction.MarkRead -> {
                    ActionRow(icon = TablerIcons.Outline.MailOpened, title = "Mark as read") {
                        val target = session
                        onDismiss()
                        AppContainer.actionScope.launch {
                            AppContainer.sessionRepository.markSessionRead(target.id)
                        }
                    }
                    SectionDivider()
                }
                ReadAction.MarkUnread -> {
                    ActionRow(icon = TablerIcons.Outline.Mail, title = "Mark as unread") {
                        val target = session
                        onDismiss()
                        AppContainer.actionScope.launch {
                            try {
                                AppContainer.sessionRepository.markSessionUnread(target.id)
                            } catch (e: Exception) {
                                confirmAlert("Failed", e.message ?: "Unable to mark as unread.")
                            }
                        }
                    }
                    SectionDivider()
                }
                null -> Unit
            }
            ActionRow(
                icon = if (isPinned) TablerIcons.Filled.Pin else TablerIcons.Outline.Pin,
                title = if (isPinned) "Unpin" else "Pin",
            ) {
                val target = session
                onDismiss()
                AppContainer.actionScope.launch {
                    try {
                        withContext(Dispatchers.IO) {
                            AppContainer.sessionRepository.updateSession(
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
            ActionRow(icon = TablerIcons.Outline.Edit, title = "Rename") { renameOpen = true }
            SectionDivider()
            ActionRow(
                icon = if (isSettled) TablerIcons.Outline.Refresh else TablerIcons.Outline.Check,
                title = if (isSettled) "Move to Active" else "Settle",
            ) {
                val target = session
                val targetSettled = !isSettled
                onDismiss()
                AppContainer.actionScope.launch {
                    try {
                        withContext(Dispatchers.IO) {
                            AppContainer.sessionRepository.settleSession(target.id, targetSettled)
                        }
                    } catch (e: Exception) {
                        confirmAlert("Failed", e.message ?: "Unable to settle chat.")
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
                                    withContext(Dispatchers.IO) { AppContainer.sessionRepository.deleteSession(session.id) }
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
                                AppContainer.sessionRepository.updateSession(session.id, UpdateSessionDto(title = renameValue.trim().ifBlank { session.title }))
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
