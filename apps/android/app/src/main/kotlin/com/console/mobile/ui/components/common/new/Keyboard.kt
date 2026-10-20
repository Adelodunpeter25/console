package com.console.mobile.ui.components.common.new

import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.nestedscroll.NestedScrollConnection
import androidx.compose.ui.input.nestedscroll.NestedScrollSource
import androidx.compose.ui.input.nestedscroll.nestedScroll
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.platform.LocalSoftwareKeyboardController

/**
 * Close the keyboard (and drop focus) as soon as the attached scrollable is
 * dragged.
 *
 * A search field at the bottom of a list leaves the keyboard covering half the
 * screen once the user stops typing; every other Android list dismisses it on
 * the first scroll, so this makes ours do the same. The filter text itself is
 * hoisted, so losing focus keeps the results.
 */
@Composable
fun Modifier.hideKeyboardOnScroll(): Modifier {
    val keyboard = LocalSoftwareKeyboardController.current
    val focus = LocalFocusManager.current
    val connection = remember(keyboard, focus) {
        object : NestedScrollConnection {
            override fun onPreScroll(available: Offset, source: NestedScrollSource): Offset {
                // Any vertical drag, including one on a list too short to scroll.
                if (available.y != 0f) {
                    keyboard?.hide()
                    focus.clearFocus()
                }
                return Offset.Zero
            }
        }
    }
    return nestedScroll(connection)
}
