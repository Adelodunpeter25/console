package com.console.mobile.feature.chat

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.ArrowUpward
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.console.mobile.AppContainer
import com.console.mobile.data.model.ImageAttachment
import com.console.mobile.ui.theme.ConsoleColors
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Chat input composer (input + send/stop + image attach + attachment strip).
 */
@Composable
fun Composer(
    sessionId: String,
    value: String,
    onChange: (String) -> Unit,
    running: Boolean,
    projectLocked: Boolean = false,
    topBanner: (@Composable () -> Unit)? = null,
    onSend: () -> Unit,
    onStop: () -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val chatSessions by AppContainer.chatStateHolder.sessions.collectAsStateWithLifecycle()
    val attachments = chatSessions[sessionId]?.attachments ?: emptyList()
    val canSend = value.trim().isNotEmpty() || attachments.isNotEmpty()

    val pickImages = rememberLauncherForActivityResult(ActivityResultContracts.GetMultipleContents()) { uris: List<Uri> ->
        if (uris.isEmpty()) return@rememberLauncherForActivityResult
        scope.launch {
            val list = withContext(Dispatchers.IO) {
                uris.take(4).mapNotNull { uri ->
                    try {
                        val mime = context.contentResolver.getType(uri) ?: "image/jpeg"
                        context.contentResolver.openInputStream(uri)?.use { ins ->
                            val bytes = ins.readBytes()
                            if (bytes.size > 10 * 1024 * 1024) return@mapNotNull null
                            ImageAttachment(data = android.util.Base64.encodeToString(bytes, android.util.Base64.NO_WRAP), mimeType = mime)
                        }
                    } catch (_: Exception) { null }
                }
            }
            if (list.isNotEmpty()) AppContainer.chatRepository.addAttachments(sessionId, list)
        }
    }

    Column(modifier = Modifier.fillMaxWidth().background(ConsoleColors.Background).padding(horizontal = 10.dp).padding(top = 8.dp, bottom = 8.dp)) {
        if (topBanner != null) topBanner()
        if (attachments.isNotEmpty()) {
            AttachmentStrip(sessionId = sessionId, attachments = attachments)
        }
        Row(
            modifier = Modifier.fillMaxWidth().clip(if (value.contains("\n")) RoundedCornerShape(20.dp) else CircleShape)
                .background(ConsoleColors.Card)
                .border(1.dp, ConsoleColors.Border, if (value.contains("\n")) RoundedCornerShape(20.dp) else CircleShape)
                .padding(horizontal = 6.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            IconButton(onClick = { pickImages.launch("image/*") }, modifier = Modifier.size(32.dp)) {
                Icon(Icons.Filled.Add, contentDescription = "Attach image", tint = ConsoleColors.TextSecondary, modifier = Modifier.size(20.dp))
            }
            BasicTextField(
                value = value,
                onValueChange = onChange,
                modifier = Modifier
                    .weight(1f)
                    .padding(horizontal = 8.dp)
                    .heightIn(max = 120.dp),
                textStyle = androidx.compose.ui.text.TextStyle(
                    color = ConsoleColors.TextPrimary,
                    fontSize = 14.sp,
                    lineHeight = 19.sp,
                ),
                cursorBrush = SolidColor(ConsoleColors.TextPrimary),
                maxLines = 6,
                decorationBox = { innerTextField ->
                    Box(contentAlignment = Alignment.CenterStart) {
                        if (value.isEmpty()) {
                            Text("Ask anything…", color = ConsoleColors.TextMuted, fontSize = 14.sp)
                        }
                        innerTextField()
                    }
                },
            )
            if (running) {
                IconButton(onClick = onStop, modifier = Modifier.size(32.dp).clip(CircleShape).background(Color.White)) {
                    Icon(Icons.Filled.Stop, contentDescription = "Stop", tint = Color.Black, modifier = Modifier.size(12.dp))
                }
            } else {
                IconButton(onClick = onSend, enabled = canSend, modifier = Modifier.size(32.dp).clip(CircleShape).background(if (canSend) Color.White else Color.White.copy(alpha = 0.08f))) {
                    Icon(Icons.Filled.ArrowUpward, contentDescription = "Send", tint = if (canSend) Color.Black else ConsoleColors.TextMuted, modifier = Modifier.size(16.dp))
                }
            }
        }
    }
}

@Composable
private fun AttachmentStrip(sessionId: String, attachments: List<ImageAttachment>) {
    Row(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(bottom = 8.dp, start = 4.dp)) {
        attachments.forEachIndexed { idx, att ->
            Box(modifier = Modifier.padding(end = 8.dp).size(56.dp).clip(RoundedCornerShape(12.dp)).background(ConsoleColors.CardAlt).border(1.dp, ConsoleColors.Border, RoundedCornerShape(12.dp))) {
                coil3.compose.AsyncImage(
                    model = "data:${att.mimeType};base64,${att.data}",
                    contentDescription = "Attachment ${idx + 1}",
                    modifier = Modifier.size(56.dp).clip(RoundedCornerShape(12.dp)),
                    contentScale = androidx.compose.ui.layout.ContentScale.Crop,
                )
                Box(modifier = Modifier.align(Alignment.TopEnd).padding(2.dp).size(18.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.6f)).clickable { AppContainer.chatRepository.removeAttachment(sessionId, idx) }, contentAlignment = Alignment.Center) {
                    Icon(Icons.Filled.Close, contentDescription = "Remove", tint = Color.White, modifier = Modifier.size(12.dp))
                }
            }
        }
    }
}
