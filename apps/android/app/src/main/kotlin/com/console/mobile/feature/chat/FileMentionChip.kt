package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.input.OffsetMapping
import androidx.compose.ui.text.input.TransformedText
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.em
import androidx.compose.ui.unit.sp
import com.console.mobile.core.util.FileMention
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleMonoFamily

/** Accent for file-mention pills, shared by the suggestion rows, composer styling and bubbles. */
internal val MentionAccent = Color(0xFF60A5FA)

/**
 * Inline file-mention pill: file-type icon + filename, accent wash.
 * Desktop parity (file_mention_chip.rs).
 */
@Composable
fun FileMentionChip(path: String, label: String = path.substringAfterLast('/')) {
    Row(
        modifier = Modifier
            .clip(RoundedCornerShape(4.dp))
            .background(MentionAccent.copy(alpha = 0.10f))
            .border(1.dp, MentionAccent.copy(alpha = 0.28f), RoundedCornerShape(4.dp))
            .padding(horizontal = 6.dp, vertical = 1.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        FileIcon(filename = label, sizeDp = 11)
        Text(
            label,
            color = MentionAccent,
            fontSize = 12.sp,
            fontFamily = ConsoleMonoFamily,
            modifier = Modifier.padding(start = 4.dp),
        )
    }
}

/**
 * Annotated bubble text with inline icon + accent spans per mention.
 * Returns the string plus the inline-content map for [Text].
 */
@Composable
fun mentionAnnotatedString(
    content: String,
    mentions: List<FileMention>,
): Pair<AnnotatedString, Map<String, InlineTextContent>> {
    val inlineContent = mentions.mapIndexed { index, mention ->
        val key = "mention-icon-$index"
        key to InlineTextContent(
            Placeholder(width = 1.1.em, height = 1.em, placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter),
        ) {
            FileIcon(filename = mention.label, sizeDp = 12)
        }
    }.toMap()
    val annotated = buildAnnotatedString {
        var cursor = 0
        mentions.forEachIndexed { index, mention ->
            val start = mention.range.first.coerceIn(0, content.length)
            val end = (mention.range.last + 1).coerceIn(start, content.length)
            append(content.substring(cursor, start))
            appendInlineContent("mention-icon-$index", "[icon]")
            withStyle(SpanStyle(color = MentionAccent, background = MentionAccent.copy(alpha = 0.10f))) {
                append(content.substring(start, end))
            }
            cursor = end
        }
        append(content.substring(cursor))
    }
    return annotated to inlineContent
}

/** Composer styling for mention ranges: accent text on a light wash. Length-preserving. */
fun mentionVisualTransformation(mentions: List<FileMention>) = VisualTransformation { text ->
    val styled = buildAnnotatedString {
        append(text.text)
        mentions.forEach { mention ->
            val start = mention.range.first.coerceIn(0, text.text.length)
            val end = (mention.range.last + 1).coerceIn(start, text.text.length)
            addStyle(
                SpanStyle(color = MentionAccent, background = MentionAccent.copy(alpha = 0.10f)),
                start,
                end,
            )
        }
    }
    TransformedText(styled, OffsetMapping.Identity)
}
