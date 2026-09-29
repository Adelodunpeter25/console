package com.console.mobile.feature.chat

import android.content.Context
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Image
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.console.mobile.AppContainer
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.components.attachmentBytes
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private const val MAX_ATTACHMENTS = 4
private const val MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024

/**
 * Picks images from the gallery, base64-encodes them off the main thread and
 * hands them to the session. Oversized or unreadable files are dropped rather
 * than failing the whole pick.
 */
@Composable
fun rememberAttachmentPicker(sessionId: String): () -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris: List<Uri> ->
        if (uris.isEmpty()) return@rememberLauncherForActivityResult
        scope.launch {
            val list = withContext(Dispatchers.IO) { uris.take(MAX_ATTACHMENTS).mapNotNull { readAttachment(context, it) } }
            if (list.isNotEmpty()) AppContainer.chatRepository.addAttachments(sessionId, list)
        }
    }
    return { launcher.launch("image/*") }
}

private fun readAttachment(context: Context, uri: Uri): ImageAttachment? = try {
    val mime = context.contentResolver.getType(uri) ?: "image/jpeg"
    context.contentResolver.openInputStream(uri)?.use { stream ->
        val bytes = stream.readBytes()
        if (bytes.size > MAX_ATTACHMENT_BYTES) null
        else ImageAttachment(data = android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP), mimeType = mime)
    }
} catch (_: Exception) {
    null
}

@Composable
fun AttachmentStrip(sessionId: String, attachments: List<ImageAttachment>) {
    var preview by remember { mutableStateOf<ImageAttachment?>(null) }
    Row(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(bottom = 8.dp, start = 4.dp)) {
        attachments.forEachIndexed { idx, att ->
            val bytes = remember(att) { attachmentBytes(att.data) }
            Box(
                modifier = Modifier.padding(end = 8.dp).size(56.dp).clip(RoundedCornerShape(12.dp))
                    .background(ConsoleColors.CardAlt)
                    .border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp))
                    .clickable { preview = att },
            ) {
                if (bytes != null) {
                    coil3.compose.AsyncImage(
                        model = bytes,
                        contentDescription = "Attachment ${idx + 1}",
                        modifier = Modifier.size(56.dp).clip(RoundedCornerShape(12.dp)),
                        contentScale = ContentScale.Crop,
                    )
                } else {
                    Box(modifier = Modifier.size(56.dp), contentAlignment = Alignment.Center) {
                        Icon(Icons.Filled.Image, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(20.dp))
                    }
                }
                Box(
                    modifier = Modifier.align(Alignment.TopEnd).padding(2.dp).size(18.dp).clip(CircleShape)
                        .background(Color.Black.copy(alpha = 0.6f))
                        .clickable { AppContainer.chatRepository.removeAttachment(sessionId, idx) },
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Filled.Close, contentDescription = "Remove", tint = Color.White, modifier = Modifier.size(12.dp))
                }
            }
        }
    }
    val current = preview
    if (current != null && attachments.contains(current)) {
        val bytes = remember(current) { attachmentBytes(current.data) }
        if (bytes != null) {
            ImagePreviewDialog(image = bytes, onDismiss = { preview = null })
        }
    }
}
