package com.console.mobile.feature.chat.markdown

import android.graphics.Rect as AndroidRect
import android.view.ActionMode
import android.view.Menu
import android.view.MenuItem
import android.view.View
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.platform.TextToolbar
import androidx.compose.ui.platform.TextToolbarStatus
import kotlin.math.roundToInt

private const val ID_REPLY = 1
private const val ID_COPY = 2

/**
 * Selection toolbar that uses the system floating action mode (the same surface Compose's
 * default toolbar uses) so it positions itself, but with our own items: Reply and Copy.
 */
internal class SelectionActionToolbar(
    private val view: View,
    private val onReply: ((String) -> Unit)?,
    private val readClipboard: () -> String,
) : TextToolbar {
    private var actionMode: ActionMode? = null
    private var rect = Rect.Zero
    private var copyAction: (() -> Unit)? = null

    override val status: TextToolbarStatus
        get() = if (actionMode != null) TextToolbarStatus.Shown else TextToolbarStatus.Hidden

    override fun hide() {
        actionMode?.finish()
        actionMode = null
        copyAction = null
    }

    override fun showMenu(
        rect: Rect,
        onCopyRequested: (() -> Unit)?,
        onPasteRequested: (() -> Unit)?,
        onCutRequested: (() -> Unit)?,
        onSelectAllRequested: (() -> Unit)?,
    ) {
        this.rect = rect
        copyAction = onCopyRequested
        val mode = actionMode
        if (mode == null) {
            actionMode = view.startActionMode(callback, ActionMode.TYPE_FLOATING)
        } else {
            mode.invalidate()
            mode.invalidateContentRect()
        }
    }

    private val callback = object : ActionMode.Callback2() {
        override fun onCreateActionMode(mode: ActionMode, menu: Menu): Boolean = true

        override fun onPrepareActionMode(mode: ActionMode, menu: Menu): Boolean {
            menu.clear()
            if (onReply != null) menu.add(Menu.NONE, ID_REPLY, 0, "Reply").setShowAsAction(MenuItem.SHOW_AS_ACTION_IF_ROOM)
            if (copyAction != null) menu.add(Menu.NONE, ID_COPY, 1, "Copy").setShowAsAction(MenuItem.SHOW_AS_ACTION_IF_ROOM)
            return true
        }

        override fun onActionItemClicked(mode: ActionMode, item: MenuItem): Boolean {
            copyAction?.invoke()
            if (item.itemId == ID_REPLY) {
                val text = readClipboard()
                if (text.isNotBlank()) onReply?.invoke(formatCiteQuote(text))
            }
            hide()
            return true
        }

        override fun onDestroyActionMode(mode: ActionMode) {
            actionMode = null
        }

        override fun onGetContentRect(mode: ActionMode, view: View, outRect: AndroidRect) {
            outRect.set(rect.left.roundToInt(), rect.top.roundToInt(), rect.right.roundToInt(), rect.bottom.roundToInt())
        }
    }
}
