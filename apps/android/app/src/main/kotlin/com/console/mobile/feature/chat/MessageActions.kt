package com.console.mobile.feature.chat

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import com.console.mobile.ui.theme.NewTheme
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Copy

/** Clipboard action shared by the user and assistant footers. */
@Composable
fun CopyButton(text: String, context: Context) {
    var copied by remember(text) { mutableStateOf(false) }
    IconButton(onClick = {
        try {
            val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
            cm.setPrimaryClip(ClipData.newPlainText("console", text))
            copied = true
        } catch (_: Exception) {}
    }, modifier = Modifier.size(24.dp)) {
        if (copied) Icon(TablerIcons.Outline.Check, contentDescription = "Copied", tint = NewTheme.Success, modifier = Modifier.size(13.dp))
        else Icon(TablerIcons.Outline.Copy, contentDescription = "Copy", tint = NewTheme.TextMuted, modifier = Modifier.size(13.dp))
    }
}
