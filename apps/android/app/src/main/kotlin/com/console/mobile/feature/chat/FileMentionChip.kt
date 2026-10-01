package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
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

/** Accent for file-mention pills, matching the desktop theme accent. */
internal val MentionAccent = Color(0xFFC85F44)

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
        FileIcon(filename = label, sizeDp = 11, modifier = Modifier.size(11.dp))
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
 * Annotated bubble text: the `@` is replaced by an inline file icon and only
 * the filename renders (accent pill), while the full `@path` stays in the
 * underlying string. Returns the string plus the inline-content map for [Text].
 */
@Composable
fun mentionAnnotatedString(
    content: String,
    mentions: List<FileMention>,
): Pair<AnnotatedString, Map<String, InlineTextContent>> {
    val inlineContent = mentions.mapIndexed { index, mention ->
        val key = "mention-icon-$index"
        key to InlineTextContent(
            Placeholder(width = 0.85.em, height = 0.85.em, placeholderVerticalAlign = PlaceholderVerticalAlign.TextCenter),
        ) {
            FileIcon(filename = mention.label, sizeDp = 12, modifier = Modifier.size(12.dp))
        }
    }.toMap()
    val annotated = buildAnnotatedString {
        var cursor = 0
        mentions.forEachIndexed { index, mention ->
            val at = mention.range.first.coerceIn(0, content.length)
            val end = (mention.range.last + 1).coerceIn(at, content.length)
            append(content.substring(cursor, at))
            appendInlineContent("mention-icon-$index", "[icon]")
            withStyle(
                SpanStyle(
                    color = MentionAccent,
                    background = MentionAccent.copy(alpha = 0.10f),
                    fontSize = 14.sp,
                )
            ) {
                append(mention.label)
            }
            cursor = end
        }
        append(content.substring(cursor))
    }
    return annotated to inlineContent
}

/**
 * One collapsed mention in transformed composer text: original `@path`
 * range plus the transformed slot (icon space) and filename range.
 */
internal data class TransformedMention(
    val oStart: Int,
    val oEnd: Int,
    val tSlot: Int,
    val tStart: Int,
    val tEnd: Int,
    val path: String,
) {
    val label: String get() = path.substringAfterLast('/')
}

/** Transformed text plus per-mention segments, shared by the styling transformation and the icon overlay. */
internal data class MentionVisual(
    val text: AnnotatedString,
    val mapping: OffsetMapping,
    val segments: List<TransformedMention>,
)

/**
 * Composer styling: each `@path` mention renders as a reserved icon slot
 * followed by just its filename in the accent pill — the `@` and parent
 * directories are hidden. Length-changing, so cursor offsets are mapped both
 * ways. The slot (a plain space at [TransformedMention.tSlot]) is where the
 * composer overlays the file icon.
 */
internal fun buildMentionVisual(original: String, mentions: List<FileMention>): MentionVisual {
    if (mentions.isEmpty() || original.isEmpty()) {
        return MentionVisual(
            AnnotatedString(original),
            OffsetMapping.Identity,
            emptyList(),
        )
    }
    val segs = ArrayList<TransformedMention>(mentions.size)
    val out = StringBuilder()
    var cursor = 0
    for (m in mentions) {
        val s = m.range.first.coerceIn(0, original.length)
        val e = (m.range.last + 1).coerceIn(s, original.length)
        if (s < cursor) continue
        out.append(original, cursor, s)
        val tSlot = out.length
        out.append(' ')
        val tStart = out.length
        out.append(m.path.substringAfterLast('/'))
        segs += TransformedMention(s, e, tSlot, tStart, out.length, m.path)
        cursor = e
    }
    out.append(original, cursor, original.length)
    val styled = buildAnnotatedString {
        append(out.toString())
        for (seg in segs) {
            addStyle(
                SpanStyle(color = MentionAccent, background = MentionAccent.copy(alpha = 0.10f)),
                seg.tStart,
                seg.tEnd,
            )
        }
    }
    val mapping = object : OffsetMapping {
        // The pill is atomic: taps inside it land on an edge, typing never
        // lands mid-path.
        override fun originalToTransformed(offset: Int): Int {
            for (seg in segs) {
                when {
                    offset < seg.oStart -> return offset + (seg.tSlot - seg.oStart)
                    offset == seg.oStart -> return seg.tSlot
                    offset < seg.oEnd -> return seg.tSlot
                    offset == seg.oEnd -> return seg.tEnd
                }
            }
            val last = segs.lastOrNull() ?: return offset
            return offset + (last.tEnd - last.oEnd)
        }

        override fun transformedToOriginal(offset: Int): Int {
            for (seg in segs) {
                when {
                    offset < seg.tSlot -> return offset - (seg.tSlot - seg.oStart)
                    offset == seg.tSlot -> return seg.oStart
                    offset < seg.tEnd -> return seg.oStart
                    offset == seg.tEnd -> return seg.oEnd
                }
            }
            val last = segs.lastOrNull() ?: return offset
            return offset - (last.tEnd - last.oEnd)
        }
    }
    return MentionVisual(styled, mapping, segs)
}

fun mentionVisualTransformation(mentions: List<FileMention>) = VisualTransformation { text ->
    val visual = buildMentionVisual(text.text, mentions)
    TransformedText(visual.text, visual.mapping)
}

/** Fixed visual (precomputed for the current buffer) as a transformation. */
internal fun MentionVisual.asTransformation() = VisualTransformation { TransformedText(text, mapping) }
