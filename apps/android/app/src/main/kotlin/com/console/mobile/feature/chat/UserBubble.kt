package com.console.mobile.feature.chat

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearOutSlowInEasing
import androidx.compose.animation.core.tween
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
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Bell
import io.github.lyxnx.compose.ui.tablericons.outline.Bolt
import io.github.lyxnx.compose.ui.tablericons.outline.Calendar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import com.console.mobile.core.util.formatMessageTime
import com.console.mobile.core.util.parseFileMentions
import com.console.mobile.data.model.BrowserAnnotationUi
import com.console.mobile.data.model.ImagePart
import com.console.mobile.ui.components.ImagePreviewDialog
import com.console.mobile.ui.components.attachmentBytes
import com.console.mobile.ui.theme.NewTheme

private fun formatSelectorLabel(selector: String): String {
    val trimmed = selector.trim()
    if (trimmed.isEmpty()) return ""
    val leaf = trimmed.substringAfterLast('>').substringAfterLast('+').substringAfterLast('~').substringAfterLast(' ').trim()
    if (leaf.startsWith('#')) return leaf
    val base = leaf.substringBefore(':')
    val pseudo = if (':' in leaf) ":" + leaf.substringAfter(':') else ""
    val parts = base.split('.').filter { it.isNotEmpty() }
    val out = StringBuilder()
    if (!base.startsWith('.')) {
        if (parts.isNotEmpty()) out.append(parts[0])
        if (parts.size > 1) out.append('.').append(parts[1])
    } else {
        if (parts.isNotEmpty()) out.append('.').append(parts[0])
    }
    out.append(pseudo)
    return if (out.isEmpty()) leaf else out.toString()
}

private const val SEND_ANIMATION_MS = 180
private val SendRise = 10.dp

// A prompt the user just sent is stamped with the local clock, while history
// carries old timestamps, so age alone separates the two without any state.
private const val JUST_SENT_WINDOW_MS = 2_000L

/** Right-aligned user message: attachments, mention text, 4-line fold, copy. */
@Composable
fun UserBubble(
    content: String,
    createdAt: Long?,
    attachments: List<ImagePart> = emptyList(),
    annotations: List<BrowserAnnotationUi> = emptyList(),
    contextFiles: List<String> = emptyList(),
    planMode: Boolean = false,
    source: String? = null,
) {
    val context = LocalContext.current
    var preview by remember { mutableStateOf<ByteArray?>(null) }
    var expanded by remember(content) { mutableStateOf(false) }
    var hasOverflow by remember(content) { mutableStateOf(false) }
    // Starts settled unless the message was sent a moment ago; remembered, so
    // scrolling a row back into view never replays it.
    val settle = remember {
        val age = createdAt?.let { System.currentTimeMillis() - it }
        Animatable(if (age != null && age in 0..JUST_SENT_WINDOW_MS) 0f else 1f)
    }
    LaunchedEffect(Unit) {
        if (settle.value < 1f) settle.animateTo(1f, tween(SEND_ANIMATION_MS, easing = LinearOutSlowInEasing))
    }
    Column(
        modifier = Modifier.fillMaxWidth().padding(bottom = 10.dp).graphicsLayer {
            alpha = settle.value
            translationY = (1f - settle.value) * SendRise.toPx()
        },
        horizontalAlignment = Alignment.End,
    ) {
        BoxWithConstraints(modifier = Modifier.align(Alignment.End)) {
            Column(
                modifier = Modifier.widthIn(min = 64.dp, max = maxWidth * 0.85f).clip(RoundedCornerShape(20.dp))
                    .background(NewTheme.UserBubble)
                    .then(if (planMode) Modifier.planModeStripes() else Modifier)
                    .padding(horizontal = 16.dp, vertical = 12.dp),
            ) {
                val sourceBadge = when (source) {
                    "automation" -> TablerIcons.Outline.Calendar to "Scheduled run"
                    "webhook" -> TablerIcons.Outline.Bolt to "Webhook event"
                    null, "" -> null
                    else -> TablerIcons.Outline.Bell to source
                }
                if (sourceBadge != null) {
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        modifier = Modifier.padding(bottom = 4.dp),
                    ) {
                        Icon(
                            sourceBadge.first,
                            contentDescription = null,
                            tint = NewTheme.Accent,
                            modifier = Modifier.size(13.dp),
                        )
                        Text(
                            sourceBadge.second,
                            color = NewTheme.Accent,
                            fontSize = 11.sp,
                            fontWeight = FontWeight.SemiBold,
                            modifier = Modifier.padding(start = 5.dp),
                        )
                    }
                }
                if (attachments.isNotEmpty() || annotations.isNotEmpty()) {
                    Row(
                        modifier = Modifier.padding(bottom = 6.dp),
                        horizontalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(8.dp),
                    ) {
                        attachments.forEach { att ->
                            val bytes = remember(att) { attachmentBytes(att.data) }
                            if (bytes != null) {
                                AsyncImage(
                                    model = bytes,
                                    contentDescription = "Attachment",
                                    modifier = Modifier
                                        .size(80.dp)
                                        .clip(RoundedCornerShape(12.dp))
                                        .clickable { preview = bytes },
                                    contentScale = ContentScale.Crop,
                                )
                            }
                        }
                        annotations.forEach { ann ->
                            val shotBytes = remember(ann.screenshotBase64) {
                                ann.screenshotBase64?.let { attachmentBytes(it) }
                            }
                            val label = remember(ann) {
                                when {
                                    ann.componentName != null && ann.sourceLocation != null -> "<${ann.componentName} />  •  ${ann.sourceLocation}"
                                    ann.componentName != null -> "<${ann.componentName} />"
                                    ann.sourceLocation != null -> ann.sourceLocation
                                    else -> formatSelectorLabel(ann.selector)
                                }
                            }
                            if (shotBytes != null) {
                                AsyncImage(
                                    model = shotBytes,
                                    contentDescription = label,
                                    modifier = Modifier
                                        .size(80.dp)
                                        .clip(RoundedCornerShape(12.dp))
                                        .clickable { preview = shotBytes },
                                    contentScale = ContentScale.Crop,
                                )
                            } else {
                                androidx.compose.foundation.layout.Box(
                                    modifier = Modifier
                                        .size(80.dp)
                                        .clip(RoundedCornerShape(12.dp))
                                        .background(NewTheme.Raised)
                                        .padding(6.dp),
                                    contentAlignment = Alignment.Center,
                                ) {
                                    Text(
                                        text = label,
                                        color = NewTheme.OnUserBubble.copy(alpha = 0.8f),
                                        fontSize = 10.5.sp,
                                        lineHeight = 14.sp,
                                        maxLines = 3,
                                        overflow = TextOverflow.Ellipsis,
                                    )
                                }
                            }
                        }
                    }
                }
                if (content.isNotEmpty()) {
                    val fileMentions = remember(content) { parseFileMentions(content) }
                    val skillMentions = remember(content, contextFiles) {
                        val parsed = com.console.mobile.core.util.parseSkillMentions(content)
                        if (contextFiles.isEmpty()) parsed else parsed.filter { "skill:${it.name}" in contextFiles }
                    }
                    val chatMentions = remember(content) { com.console.mobile.core.util.parseChatMentions(content) }
                    val maxLines = if (expanded) Int.MAX_VALUE else 4
                    if (fileMentions.isEmpty() && skillMentions.isEmpty() && chatMentions.isEmpty()) {
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
                        val (annotated, inlineContent) = mentionAnnotatedString(content, fileMentions, skillMentions, chatMentions)
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
    if (current != null) {
        ImagePreviewDialog(image = current, onDismiss = { preview = null })
    }
}
