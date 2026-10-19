package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import com.console.mobile.core.util.formatMessageTime
import com.console.mobile.core.util.parseFileMentions
import com.console.mobile.data.model.ImagePart
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.components.attachmentBytes
import com.console.mobile.ui.theme.NewTheme

/** Right-aligned user message: attachments, mention text, 4-line fold, copy. */
@Composable
fun UserBubble(content: String, createdAt: Long?, attachments: List<ImagePart> = emptyList()) {
    val context = LocalContext.current
    var preview by remember { mutableStateOf<ImagePart?>(null) }
    var expanded by remember(content) { mutableStateOf(false) }
    var hasOverflow by remember(content) { mutableStateOf(false) }
    Column(modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp), horizontalAlignment = Alignment.End) {
        BoxWithConstraints(modifier = Modifier.align(Alignment.End)) {
            Column(
                modifier = Modifier.widthIn(min = 64.dp, max = maxWidth * 0.85f).clip(RoundedCornerShape(20.dp))
                    .background(NewTheme.UserBubble).padding(horizontal = 16.dp, vertical = 12.dp),
            ) {
                if (attachments.isNotEmpty()) {
                    Row(modifier = Modifier.padding(bottom = 6.dp)) {
                        attachments.forEach { att ->
                            val bytes = remember(att) { attachmentBytes(att.data) }
                            if (bytes != null) {
                                AsyncImage(model = bytes, contentDescription = "Attachment", modifier = Modifier.size(80.dp).clip(RoundedCornerShape(12.dp)).clickable { preview = att }, contentScale = ContentScale.Crop)
                            }
                        }
                    }
                }
                if (content.isNotEmpty()) {
                    val mentions = remember(content) { parseFileMentions(content) }
                    val maxLines = if (expanded) Int.MAX_VALUE else 4
                    if (mentions.isEmpty()) {
                        Text(
                            content,
                            color = NewTheme.OnUserBubble,
                            fontSize = 16.sp,
                            lineHeight = 23.sp,
                            maxLines = maxLines,
                            overflow = TextOverflow.Ellipsis,
                            onTextLayout = { textLayoutResult ->
                                if (!expanded) {
                                    hasOverflow = textLayoutResult.hasVisualOverflow || textLayoutResult.lineCount > 4
                                }
                            },
                        )
                    } else {
                        val (annotated, inlineContent) = mentionAnnotatedString(content, mentions)
                        Text(
                            annotated,
                            inlineContent = inlineContent,
                            color = NewTheme.OnUserBubble,
                            fontSize = 16.sp,
                            lineHeight = 23.sp,
                            maxLines = maxLines,
                            overflow = TextOverflow.Ellipsis,
                            onTextLayout = { textLayoutResult ->
                                if (!expanded) {
                                    hasOverflow = textLayoutResult.hasVisualOverflow || textLayoutResult.lineCount > 4
                                }
                            },
                        )
                    }
                    if (hasOverflow) {
                        Text(
                            text = if (expanded) "Show less" else "Show more",
                            color = NewTheme.Accent,
                            fontSize = 13.sp,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier
                                .padding(top = 6.dp)
                                .clickable { expanded = !expanded },
                        )
                    }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 4.dp, end = 2.dp)) {
            Text(formatMessageTime(createdAt ?: System.currentTimeMillis()), color = NewTheme.TextMuted, fontSize = 12.sp)
            if (content.isNotEmpty()) {
                CopyButton(text = content, context = context)
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
