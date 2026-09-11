package com.console.mobile.feature.chat

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * Lightweight markdown renderer (mikepenz markdown-renderer-m3 is wired for the
 * full ChatScreen pass; this covers bubbles/tool results/diffs meanwhile).
 * Supports: fenced code (header + copy + mono + h-scroll), inline code,
 * bold/italic/strike, headings, blockquote, bullets/ordered, hr, links (decorated).
 */
@Composable
fun MarkdownText(content: String, modifier: Modifier = Modifier, streaming: Boolean = false) {
    if (content.isBlank()) return
    SelectionContainer(modifier = modifier) {
        Column(modifier = Modifier.fillMaxWidth()) {
            parseBlocks(content).forEach { block ->
                when (block) {
                    is MdBlock.Code -> CodeBlock(language = block.language, code = block.code)
                    is MdBlock.Heading -> Text(
                        block.text,
                        color = ConsoleColors.TextPrimary,
                        fontSize = when (block.level) { 1 -> 18.sp; 2 -> 16.sp; else -> 15.sp },
                        fontWeight = FontWeight.Bold,
                        modifier = Modifier.padding(top = 8.dp, bottom = 4.dp),
                    )
                    is MdBlock.Quote -> Box(
                        modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp).clip(RoundedCornerShape(topEnd = 12.dp, bottomEnd = 12.dp))
                            .background(Color.White.copy(alpha = 0.05f)).padding(horizontal = 14.dp, vertical = 8.dp),
                    ) {
                        InlineText(block.text, 14.sp, ConsoleColors.TextSecondary)
                    }
                    is MdBlock.Rule -> Box(modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp).background(Color.White.copy(alpha = 0.1f)).padding(vertical = 0.5.dp))
                    is MdBlock.Bullets -> Column(modifier = Modifier.padding(vertical = 2.dp)) {
                        block.items.forEach { item ->
                            Row(modifier = Modifier.padding(vertical = 2.dp)) {
                                Text("•  ", color = ConsoleColors.TextSecondary, fontSize = 14.sp)
                                InlineText(item, 14.sp, ConsoleColors.TextPrimary)
                            }
                        }
                    }
                    is MdBlock.Ordered -> Column(modifier = Modifier.padding(vertical = 2.dp)) {
                        block.items.forEachIndexed { i, item ->
                            Row(modifier = Modifier.padding(vertical = 2.dp)) {
                                Text("${i + 1}.  ", color = ConsoleColors.TextSecondary, fontSize = 14.sp)
                                InlineText(item, 14.sp, ConsoleColors.TextPrimary)
                            }
                        }
                    }
                    is MdBlock.Paragraph -> InlineText(block.text, 15.sp, ConsoleColors.TextPrimary, lineHeight = 22.sp)
                }
            }
            if (streaming) {
                Text("▍", color = ConsoleColors.TextMuted, fontSize = 14.sp)
            }
        }
    }
}

@Composable
private fun CodeBlock(language: String, code: String) {
    val clipboard = LocalClipboardManager.current
    val scope = rememberCoroutineScope()
    var copied by remember(code) { mutableStateOf(false) }
    val shape = RoundedCornerShape(12.dp)
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 10.dp).clip(shape)
            .background(Color(0xFF101113))
            .border(1.dp, Color.White.copy(alpha = 0.1f), shape),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            com.console.mobile.ui.components.FileLanguageIcon(language = language, sizeDp = 13)
            Text(language.ifBlank { "code" }, color = ConsoleColors.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 6.dp))
            IconButton(onClick = {
                clipboard.setText(AnnotatedString(code))
                copied = true
                scope.launch { delay(1500); copied = false }
            }, modifier = Modifier.padding(0.dp)) {
                if (copied) Icon(Icons.Filled.Check, contentDescription = "Copied", tint = Color(0xFF34D399))
                else Icon(Icons.Filled.ContentCopy, contentDescription = "Copy code", tint = ConsoleColors.TextSecondary)
            }
        }
        Box(modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 14.dp).padding(bottom = 12.dp)) {
            Text(code.trimEnd(), color = Color(0xFFE4E4E7), fontSize = 12.5.sp, fontFamily = ConsoleMonoFamily, lineHeight = 19.sp)
        }
    }
}

@Composable
private fun InlineText(text: String, size: androidx.compose.ui.unit.TextUnit, color: Color, lineHeight: androidx.compose.ui.unit.TextUnit = androidx.compose.ui.unit.TextUnit.Unspecified) {
    Text(buildInline(text, color), fontSize = size, lineHeight = lineHeight, modifier = Modifier.fillMaxWidth())
}

private fun buildInline(src: String, base: Color): AnnotatedString {
    // Order: inline code, bold, italic, strike, links.
    val out = buildAnnotatedString {
        var i = 0
        pushStyle(SpanStyle(color = base))
        while (i < src.length) {
            when {
                src.startsWith("`", i) -> {
                    val end = src.indexOf("`", i + 1)
                    if (end == -1) { append(src.substring(i)); break }
                    pushStyle(SpanStyle(color = Color(0xFFFDBA74), fontFamily = ConsoleMonoFamily, fontSize = 13.sp, background = Color.White.copy(alpha = 0.08f)))
                    append(src.substring(i + 1, end))
                    pop()
                    i = end + 1
                }
                src.startsWith("**", i) -> {
                    val end = src.indexOf("**", i + 2)
                    if (end == -1) { append(src.substring(i)); break }
                    pushStyle(SpanStyle(fontWeight = FontWeight.Bold, color = Color.White))
                    appendInlineRaw(src.substring(i + 2, end))
                    pop()
                    i = end + 2
                }
                src.startsWith("~~", i) -> {
                    val end = src.indexOf("~~", i + 2)
                    if (end == -1) { append(src.substring(i)); break }
                    pushStyle(SpanStyle(textDecoration = TextDecoration.LineThrough))
                    append(src.substring(i + 2, end))
                    pop()
                    i = end + 2
                }
                src.startsWith("*", i) && !src.startsWith("**", i) -> {
                    val end = src.indexOf("*", i + 1)
                    if (end == -1) { append(src.substring(i)); break }
                    pushStyle(SpanStyle(fontStyle = FontStyle.Italic))
                    append(src.substring(i + 1, end))
                    pop()
                    i = end + 1
                }
                src.startsWith("[", i) -> {
                    val mid = src.indexOf("](", i)
                    val end = if (mid != -1) src.indexOf(")", mid) else -1
                    if (mid == -1 || end == -1) { append(src[i]); i++ }
                    else {
                        pushStyle(SpanStyle(color = Color(0xFF7DD3FC), textDecoration = TextDecoration.Underline))
                        append(src.substring(i + 1, mid))
                        pop()
                        i = end + 1
                    }
                }
                else -> { append(src[i]); i++ }
            }
        }
        pop()
    }
    return out
}

private fun androidx.compose.ui.text.AnnotatedString.Builder.appendInlineRaw(s: String) {
    // Nested emphasis inside bold — reuse code-span handling only.
    var i = 0
    while (i < s.length) {
        if (s[i] == '`') {
            val end = s.indexOf('`', i + 1)
            if (end == -1) { append(s.substring(i)); break }
            pushStyle(SpanStyle(fontFamily = FontFamily.Monospace))
            append(s.substring(i + 1, end))
            pop()
            i = end + 1
        } else { append(s[i]); i++ }
    }
}

private sealed interface MdBlock {
    data class Code(val language: String, val code: String) : MdBlock
    data class Heading(val level: Int, val text: String) : MdBlock
    data class Quote(val text: String) : MdBlock
    data object Rule : MdBlock
    data class Bullets(val items: List<String>) : MdBlock
    data class Ordered(val items: List<String>) : MdBlock
    data class Paragraph(val text: String) : MdBlock
}

private fun parseBlocks(src: String): List<MdBlock> {
    val blocks = mutableListOf<MdBlock>()
    val lines = src.split("\n")
    var i = 0
    val para = StringBuilder()
    fun flushPara() {
        val t = para.toString().trim()
        if (t.isNotEmpty()) blocks.add(MdBlock.Paragraph(t))
        para.clear()
    }
    while (i < lines.size) {
        val line = lines[i]
        val trimmed = line.trim()
        if (trimmed.startsWith("```")) {
            flushPara()
            val lang = trimmed.removePrefix("```").trim()
            val code = StringBuilder()
            i++
            while (i < lines.size && !lines[i].trim().startsWith("```")) {
                code.appendLine(lines[i]); i++
            }
            blocks.add(MdBlock.Code(lang, code.toString()))
            i++
            continue
        }
        if (trimmed.startsWith("#")) {
            flushPara()
            val level = trimmed.takeWhile { it == '#' }.length.coerceIn(1, 3)
            blocks.add(MdBlock.Heading(level, trimmed.drop(level).trim()))
            i++; continue
        }
        if (trimmed == "---" || trimmed == "***" || trimmed == "___") {
            flushPara(); blocks.add(MdBlock.Rule); i++; continue
        }
        if (trimmed.startsWith(">")) {
            flushPara()
            val q = StringBuilder()
            while (i < lines.size && lines[i].trim().startsWith(">")) {
                q.append(lines[i].trim().removePrefix(">").trim()); q.append("\n"); i++
            }
            blocks.add(MdBlock.Quote(q.toString().trim()))
            continue
        }
        val bulletMatch = Regex("^\\s*[-*+] ").containsMatchIn(line)
        if (bulletMatch) {
            flushPara()
            val items = mutableListOf<String>()
            while (i < lines.size && Regex("^\\s*[-*+] ").containsMatchIn(lines[i])) {
                items.add(lines[i].trim().drop(2).trim()); i++
            }
            blocks.add(MdBlock.Bullets(items))
            continue
        }
        val orderedMatch = Regex("^\\s*\\d+[.)] ").containsMatchIn(line)
        if (orderedMatch) {
            flushPara()
            val items = mutableListOf<String>()
            while (i < lines.size && Regex("^\\s*\\d+[.)] ").containsMatchIn(lines[i])) {
                items.add(lines[i].trim().replaceFirst(Regex("^\\d+[.)]\\s*"), "")); i++
            }
            blocks.add(MdBlock.Ordered(items))
            continue
        }
        if (trimmed.isEmpty()) { flushPara(); i++; continue }
        // Hard-break: two trailing spaces or single newline inside paragraph.
        if (para.isNotEmpty()) para.append("\n")
        para.append(line.trimEnd())
        i++
    }
    flushPara()
    return blocks
}
