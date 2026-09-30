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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Photo
import io.github.lyxnx.compose.ui.tablericons.outline.X
import com.console.mobile.AppContainer
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.data.model.newAttachmentId
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Per-file memory guard, not a cap on how many you can attach. */
private const val MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024

/** Thumbnails shown before the strip collapses the rest behind a "+N" card. */
private const val VISIBLE_ATTACHMENTS = 4

/**
 * Picks images from the gallery and hands them to the session as raw bytes.
 * There is no count cap — the strip paginates instead. A file too large to hold
 * is dropped rather than failing the whole pick; everything else is kept.
 */
@Composable
fun rememberAttachmentPicker(sessionId: String): () -> Unit {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris: List<Uri> ->
        if (uris.isEmpty()) return@rememberLauncherForActivityResult
        scope.launch {
            val list = withContext(Dispatchers.IO) { uris.mapNotNull { readAttachment(context, it) } }
            if (list.isNotEmpty()) AppContainer.chatRepository.addAttachments(sessionId, list)
        }
    }
    return { launcher.launch("image/*") }
}

private fun readAttachment(context: Context, uri: Uri): ImageAttachment? = try {
    val mime = context.contentResolver.getType(uri) ?: "image/jpeg"
    context.contentResolver.openInputStream(uri)?.use { stream ->
        val bytes = stream.readBytes()
        // A memory guard, not a product limit: one oversized photo should not
        // take the app down, and the user can still attach everything else.
        if (bytes.size > MAX_ATTACHMENT_BYTES) null
        else ImageAttachment(id = newAttachmentId(), bytes = bytes, mimeType = mime)
    }
} catch (_: Exception) {
    null
}

/**
 * The thumbnail strip above the input. Shows at most [VISIBLE_ATTACHMENTS]
 * cards; anything beyond that collapses into a trailing "+N" card that expands
 * the strip to the next level. Tapping a thumbnail opens the full preview,
 * its ✕ removes just that image.
 */
@Composable
fun AttachmentStrip(sessionId: String, attachments: List<ImageAttachment>) {
    var preview by remember { mutableStateOf<ImageAttachment?>(null) }
    // Two levels only: the collapsed strip, and everything expanded. Collapsing
    // again happens by removing images, not by a third level.
    var expanded by remember { mutableStateOf(false) }
    val visible = if (expanded) attachments else attachments.take(VISIBLE_ATTACHMENTS)
    val hidden = attachments.size - visible.size

    Row(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(bottom = 8.dp, start = 4.dp)) {
        visible.forEachIndexed { idx, att ->
            AttachmentCard(
                attachment = att,
                onClick = { preview = att },
                onRemove = { AppContainer.chatRepository.removeAttachment(sessionId, idx) },
            )
        }
        if (hidden > 0) {
            OverflowCard(count = hidden, onClick = { expanded = true })
        }
    }

    val current = preview
    if (current != null && attachments.contains(current)) {
        ImagePreviewDialog(image = current.bytes, onDismiss = { preview = null })
    }
}

@Composable
private fun AttachmentCard(attachment: ImageAttachment, onClick: () -> Unit, onRemove: () -> Unit) {
    val shape = RoundedCornerShape(12.dp)
    Box(
        modifier = Modifier.padding(end = 8.dp).size(56.dp).clip(shape)
            .background(ConsoleColors.CardAlt)
            .border(1.dp, ConsoleColors.Border, shape)
            .clickable(onClick = onClick),
    ) {
        if (attachment.bytes.isNotEmpty()) {
            coil3.compose.AsyncImage(
                model = attachment.bytes,
                contentDescription = "Attachment",
                modifier = Modifier.size(56.dp).clip(shape),
                contentScale = ContentScale.Crop,
            )
        } else {
            Box(modifier = Modifier.size(56.dp), contentAlignment = Alignment.Center) {
                Icon(TablerIcons.Outline.Photo, contentDescription = null, tint = ConsoleColors.TextMuted, modifier = Modifier.size(20.dp))
            }
        }
        Box(
            modifier = Modifier.align(Alignment.TopEnd).padding(2.dp).size(18.dp).clip(CircleShape)
                .background(Color.Black.copy(alpha = 0.6f))
                .clickable(onClick = onRemove),
            contentAlignment = Alignment.Center,
        ) {
            Icon(TablerIcons.Outline.X, contentDescription = "Remove", tint = Color.White, modifier = Modifier.size(12.dp))
        }
    }
}

/** Trailing "+N" card that reveals the rest of the attachments. */
@Composable
private fun OverflowCard(count: Int, onClick: () -> Unit) {
    Box(
        modifier = Modifier.size(56.dp).clip(RoundedCornerShape(12.dp))
            .background(ConsoleColors.SurfaceElevated)
            .border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp))
            .clickable(onClickLabel = "Show $count more", onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        Text("+$count", color = ConsoleColors.TextSecondary, fontSize = 14.sp, fontWeight = FontWeight.Medium)
    }
}
