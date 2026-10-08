package com.console.mobile.feature.chat.markdown

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
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
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.code.CodeViewer
import com.console.mobile.ui.theme.NewTheme
import com.console.mobile.ui.theme.ConsoleMonoFamily
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.Check
import io.github.lyxnx.compose.ui.tablericons.outline.Copy
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.intellij.markdown.IElementType
import org.intellij.markdown.MarkdownElementTypes
import org.intellij.markdown.MarkdownTokenTypes
import org.intellij.markdown.ast.ASTNode
import org.intellij.markdown.flavours.gfm.GFMElementTypes
import org.intellij.markdown.flavours.gfm.GFMFlavourDescriptor
import org.intellij.markdown.flavours.gfm.GFMTokenTypes
import org.intellij.markdown.parser.MarkdownParser

/**
 * Markdown renderer. Parsing is done by JetBrains `markdown` (CommonMark + GFM);
 * this file converts its AST into our own block model and draws it with Compose.
 * Supports: fenced/indented code (header + copy), inline code, bold/italic/strike,
 * headings, block quotes, nested bullet/ordered/task lists, tables, rules, links.
 */
@Composable
fun CustomMarkdown(content: String, modifier: Modifier = Modifier, streaming: Boolean = false) {
    if (content.isBlank()) return
    SelectionContainer(modifier = modifier) {
        val blocks = remember(content) { parseBlocks(content) }
        Column(modifier = Modifier.fillMaxWidth()) {
            MdBlocks(blocks, NewTheme.TextPrimary, 15.sp, 22.sp, 6.dp)
            if (streaming) {
                Text("▍", color = NewTheme.TextMuted, fontSize = 14.sp)
            }
        }
    }
}

// ───────────────────────────── Rendering ─────────────────────────────

@Composable
private fun MdBlocks(blocks: List<MdBlock>, color: Color, size: TextUnit, lineHeight: TextUnit, gap: Dp) {
    blocks.forEachIndexed { index, block ->
        key(index) { MdBlockView(block, color, size, lineHeight, gap) }
    }
}

@Composable
private fun MdBlockView(block: MdBlock, color: Color, size: TextUnit, lineHeight: TextUnit, gap: Dp) {
    when (block) {
        is MdBlock.Code -> CodeBlock(language = block.language, code = block.code)
        is MdBlock.Heading -> Text(
            block.text,
            color = NewTheme.TextPrimary,
            fontSize = when (block.level) { 1 -> 18.sp; 2 -> 16.sp; else -> 15.sp },
            fontWeight = FontWeight.Bold,
            modifier = Modifier.padding(top = 8.dp, bottom = 4.dp),
        )
        is MdBlock.Quote -> Box(
            modifier = Modifier.fillMaxWidth().padding(vertical = 6.dp).clip(RoundedCornerShape(topEnd = 12.dp, bottomEnd = 12.dp))
                .background(Color.White.copy(alpha = 0.05f)).padding(horizontal = 14.dp, vertical = 8.dp),
        ) {
            Column {
                MdBlocks(block.blocks, NewTheme.TextSecondary, 14.sp, TextUnit.Unspecified, 4.dp)
            }
        }
        is MdBlock.Rule -> Box(modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp).background(Color.White.copy(alpha = 0.1f)).padding(vertical = 0.5.dp))
        is MdBlock.ListBlock -> ListView(block, color)
        is MdBlock.Table -> TableView(block)
        is MdBlock.Paragraph -> Text(
            block.text,
            color = color,
            fontSize = size,
            lineHeight = lineHeight,
            modifier = Modifier.fillMaxWidth().padding(bottom = gap),
        )
    }
}

@Composable
private fun ListView(block: MdBlock.ListBlock, color: Color) {
    Column(modifier = Modifier.padding(vertical = 2.dp)) {
        block.items.forEachIndexed { i, item ->
            val checked = item.checked
            val marker = when {
                checked != null -> if (checked) "☑" else "☐"
                block.ordered -> "${block.start + i}."
                else -> "•"
            }
            Row(modifier = Modifier.padding(vertical = 2.dp)) {
                Text(
                    marker,
                    color = NewTheme.TextSecondary,
                    fontSize = 14.sp,
                    modifier = Modifier.widthIn(min = 20.dp).padding(end = 6.dp),
                )
                Column(modifier = Modifier.weight(1f)) {
                    MdBlocks(item.blocks, color, 14.sp, TextUnit.Unspecified, 0.dp)
                }
            }
        }
    }
}

@Composable
private fun TableView(block: MdBlock.Table) {
    // Column width from the longest cell (rough 8dp/char), clamped so wide tables scroll sideways.
    val widths = remember(block) {
        val cols = maxOf(block.header.size, block.rows.maxOfOrNull { it.size } ?: 0)
        List(cols) { c ->
            val longest = maxOf(
                block.header.getOrNull(c)?.length ?: 0,
                block.rows.maxOfOrNull { it.getOrNull(c)?.length ?: 0 } ?: 0,
            )
            (longest * 8 + 24).coerceIn(72, 260).dp
        }
    }
    val shape = RoundedCornerShape(8.dp)
    Column(
        modifier = Modifier.padding(vertical = 8.dp)
            .horizontalScroll(rememberScrollState())
            .clip(shape)
            .border(1.dp, NewTheme.Divider, shape),
    ) {
        TableRowView(block.header, widths, header = true)
        block.rows.forEach { TableRowView(it, widths, header = false) }
    }
}

@Composable
private fun TableRowView(cells: List<AnnotatedString>, widths: List<Dp>, header: Boolean) {
    Row(
        modifier = Modifier.height(IntrinsicSize.Min)
            .background(if (header) NewTheme.Raised else Color.Transparent),
    ) {
        widths.forEachIndexed { c, w ->
            Text(
                cells.getOrNull(c) ?: AnnotatedString(""),
                color = NewTheme.TextPrimary,
                fontSize = 13.sp,
                fontWeight = if (header) FontWeight.SemiBold else null,
                modifier = Modifier.width(w).fillMaxHeight()
                    .border(0.5.dp, NewTheme.Divider)
                    .padding(horizontal = 10.dp, vertical = 7.dp),
            )
        }
    }
}

@Composable
private fun CodeBlock(language: String, code: String) {
    val clipboard = LocalClipboardManager.current
    val scope = rememberCoroutineScope()
    var copied by remember(code) { mutableStateOf(false) }
    val capped = remember(code) { code.trimEnd() }
    val normalizedLang = remember(language) { language.trim().lowercase() }
    // Fixed height so the Sora view can virtualize inside chat scroll; capped with internal scroll.
    val lineCount = remember(capped) { capped.count { it == '\n' } + 1 }
    val viewerHeight = remember(lineCount) { ((lineCount * 20 + 16).coerceAtMost(440)).dp }
    Column(
        modifier = Modifier.fillMaxWidth().padding(vertical = 10.dp),
    ) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            com.console.mobile.ui.components.FileLanguageIcon(language = language, sizeDp = 13)
            Text(language.ifBlank { "code" }, color = NewTheme.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 6.dp))
            IconButton(onClick = {
                clipboard.setText(AnnotatedString(code))
                copied = true
                scope.launch { delay(1500); copied = false }
            }, modifier = Modifier.padding(0.dp)) {
                if (copied) Icon(TablerIcons.Outline.Check, contentDescription = "Copied", tint = Color(0xFF34D399))
                else Icon(TablerIcons.Outline.Copy, contentDescription = "Copy code", tint = NewTheme.TextSecondary)
            }
        }
        CodeViewer(
            code = capped,
            language = normalizedLang,
            modifier = Modifier.fillMaxWidth().height(viewerHeight).padding(horizontal = 14.dp).padding(bottom = 12.dp),
            showLineNumbers = true,
        )
    }
}

// ───────────────────────────── Block model ─────────────────────────────

// @Immutable: blocks are never mutated after parsing, so Compose can skip unchanged blocks.
@Immutable
private sealed interface MdBlock {
    @Immutable data class Code(val language: String, val code: String) : MdBlock
    @Immutable data class Heading(val level: Int, val text: AnnotatedString) : MdBlock
    @Immutable data class Quote(val blocks: List<MdBlock>) : MdBlock
    data object Rule : MdBlock
    @Immutable data class ListBlock(val ordered: Boolean, val start: Int, val items: List<MdListItem>) : MdBlock
    @Immutable data class Paragraph(val text: AnnotatedString) : MdBlock
    @Immutable data class Table(val header: List<AnnotatedString>, val rows: List<List<AnnotatedString>>) : MdBlock
}

/** [checked] is null for a normal item, true/false for a `- [x]` / `- [ ]` task item. */
@Immutable
private data class MdListItem(val checked: Boolean?, val blocks: List<MdBlock>)

// ───────────────────────────── AST → blocks ─────────────────────────────

// Parser is stateless between calls, so one shared instance is fine.
private val PARSER = MarkdownParser(GFMFlavourDescriptor())

private val ATX_LEVELS = mapOf(
    MarkdownElementTypes.ATX_1 to 1,
    MarkdownElementTypes.ATX_2 to 2,
    MarkdownElementTypes.ATX_3 to 3,
    MarkdownElementTypes.ATX_4 to 4,
    MarkdownElementTypes.ATX_5 to 5,
    MarkdownElementTypes.ATX_6 to 6,
)

private fun parseBlocks(src: String): List<MdBlock> =
    blocksOf(PARSER.buildMarkdownTreeFromString(src).children, src)

private fun ASTNode.text(src: String): String = src.substring(startOffset, endOffset)

private fun blocksOf(nodes: List<ASTNode>, src: String): List<MdBlock> {
    val out = ArrayList<MdBlock>(nodes.size)
    for (n in nodes) {
        when (n.type) {
            MarkdownElementTypes.PARAGRAPH -> {
                val text = inlineOf(n.children, src)
                if (text.text.isNotBlank()) out.add(MdBlock.Paragraph(text))
            }
            MarkdownElementTypes.ATX_1, MarkdownElementTypes.ATX_2, MarkdownElementTypes.ATX_3,
            MarkdownElementTypes.ATX_4, MarkdownElementTypes.ATX_5, MarkdownElementTypes.ATX_6 ->
                headingBlock(n, MarkdownTokenTypes.ATX_CONTENT, ATX_LEVELS.getValue(n.type), src)?.let { out.add(it) }
            MarkdownElementTypes.SETEXT_1 -> headingBlock(n, MarkdownTokenTypes.SETEXT_CONTENT, 1, src)?.let { out.add(it) }
            MarkdownElementTypes.SETEXT_2 -> headingBlock(n, MarkdownTokenTypes.SETEXT_CONTENT, 2, src)?.let { out.add(it) }
            MarkdownElementTypes.CODE_FENCE -> out.add(fenceBlock(n, src))
            MarkdownElementTypes.CODE_BLOCK -> out.add(indentedCodeBlock(n, src))
            MarkdownElementTypes.BLOCK_QUOTE -> out.add(MdBlock.Quote(blocksOf(n.children, src)))
            MarkdownElementTypes.UNORDERED_LIST, MarkdownElementTypes.ORDERED_LIST -> out.add(listBlock(n, src))
            MarkdownTokenTypes.HORIZONTAL_RULE -> out.add(MdBlock.Rule)
            GFMElementTypes.TABLE -> out.add(tableBlock(n, src))
            // Raw HTML / math blocks: show the source rather than dropping it.
            MarkdownElementTypes.HTML_BLOCK, GFMElementTypes.BLOCK_MATH -> {
                val raw = n.text(src).trim()
                if (raw.isNotEmpty()) out.add(MdBlock.Paragraph(AnnotatedString(raw)))
            }
            // EOL, whitespace, link definitions, block-quote markers, ...: nothing to draw.
        }
    }
    return out
}

private fun headingBlock(n: ASTNode, contentType: IElementType, level: Int, src: String): MdBlock? {
    val content = n.children.firstOrNull { it.type == contentType } ?: return null
    return MdBlock.Heading(level.coerceAtMost(3), inlineOf(content.children, src))
}

/** Fenced code: content lines joined, minus the fence's own indentation (e.g. inside a list item). */
private fun fenceBlock(n: ASTNode, src: String): MdBlock.Code {
    var lang = ""
    var indent = 0
    var openerEolSeen = false
    val sb = StringBuilder()
    for (c in n.children) {
        when (c.type) {
            MarkdownTokenTypes.CODE_FENCE_START -> {
                val t = c.text(src)
                indent = t.length - t.trimStart().length
            }
            MarkdownTokenTypes.FENCE_LANG -> lang = c.text(src).trim().substringBefore(' ')
            // The first EOL ends the opening-fence line; later ones separate code lines.
            MarkdownTokenTypes.EOL -> if (openerEolSeen) sb.append('\n') else openerEolSeen = true
            MarkdownTokenTypes.CODE_FENCE_CONTENT -> sb.append(stripIndent(c.text(src), indent))
        }
    }
    return MdBlock.Code(lang, sb.toString())
}

private fun indentedCodeBlock(n: ASTNode, src: String): MdBlock.Code {
    val sb = StringBuilder()
    for (c in n.children) {
        when (c.type) {
            MarkdownTokenTypes.CODE_LINE -> sb.append(stripIndent(c.text(src), 4))
            MarkdownTokenTypes.EOL -> sb.append('\n')
        }
    }
    return MdBlock.Code("", sb.toString())
}

/** Remove up to [n] leading spaces. */
private fun stripIndent(line: String, n: Int): String {
    var k = 0
    while (k < n && k < line.length && line[k] == ' ') k++
    return line.substring(k)
}

private fun listBlock(n: ASTNode, src: String): MdBlock.ListBlock {
    val ordered = n.type == MarkdownElementTypes.ORDERED_LIST
    var start = 1
    val items = ArrayList<MdListItem>()
    for (item in n.children) {
        if (item.type != MarkdownElementTypes.LIST_ITEM) continue
        if (ordered && items.isEmpty()) {
            val number = item.children.firstOrNull { it.type == MarkdownTokenTypes.LIST_NUMBER }
            start = number?.text(src)?.trim()?.takeWhile { it.isDigit() }?.toIntOrNull() ?: 1
        }
        val box = item.children.firstOrNull { it.type == GFMTokenTypes.CHECK_BOX }
        val checked = box?.text(src)?.contains('x', ignoreCase = true)
        items.add(MdListItem(checked, blocksOf(item.children, src)))
    }
    return MdBlock.ListBlock(ordered, start, items)
}

private fun tableBlock(n: ASTNode, src: String): MdBlock.Table {
    var header: List<AnnotatedString> = emptyList()
    val rows = ArrayList<List<AnnotatedString>>()
    for (c in n.children) {
        when (c.type) {
            GFMElementTypes.HEADER -> header = tableCells(c, src)
            GFMElementTypes.ROW -> rows.add(tableCells(c, src))
        }
    }
    return MdBlock.Table(header, rows)
}

private fun tableCells(row: ASTNode, src: String): List<AnnotatedString> =
    row.children.filter { it.type == GFMTokenTypes.CELL }.map { inlineOf(it.children, src) }

// ───────────────────────────── Inline nodes → AnnotatedString ─────────────────────────────

// Shared span styles: allocated once instead of per span.
private val CODE_SPAN = SpanStyle(color = Color(0xFFFDBA74), fontFamily = ConsoleMonoFamily, fontSize = 13.sp, background = Color.White.copy(alpha = 0.08f))
private val BOLD_SPAN = SpanStyle(fontWeight = FontWeight.Bold, color = Color.White)
private val STRIKE_SPAN = SpanStyle(textDecoration = TextDecoration.LineThrough)
private val ITALIC_SPAN = SpanStyle(fontStyle = FontStyle.Italic)
private val LINK_STYLES = TextLinkStyles(style = SpanStyle(color = Color(0xFF7DD3FC), textDecoration = TextDecoration.Underline))

private fun inlineOf(nodes: List<ASTNode>, src: String): AnnotatedString =
    buildAnnotatedString { appendInline(trimEdges(nodes), src, linked = false) }

private fun trimEdges(nodes: List<ASTNode>): List<ASTNode> {
    fun blank(n: ASTNode) = n.type == MarkdownTokenTypes.WHITE_SPACE || n.type == MarkdownTokenTypes.EOL
    var s = 0
    var e = nodes.size
    while (s < e && blank(nodes[s])) s++
    while (e > s && blank(nodes[e - 1])) e--
    return nodes.subList(s, e)
}

private inline fun AnnotatedString.Builder.styled(style: SpanStyle, block: () -> Unit) {
    pushStyle(style)
    block()
    pop()
}

private inline fun AnnotatedString.Builder.link(url: String, block: () -> Unit) {
    if (url.isBlank()) { block(); return }
    pushLink(LinkAnnotation.Url(url, LINK_STYLES))
    block()
    pop()
}

/** Children of an emphasis-like node without its `*` / `_` / `~` marker tokens. */
private fun withoutMarkers(nodes: List<ASTNode>): List<ASTNode> =
    nodes.filter { it.type != MarkdownTokenTypes.EMPH && it.type != GFMTokenTypes.TILDE }

/** [linked]: already inside a link, so don't nest another link annotation. */
private fun AnnotatedString.Builder.appendInline(nodes: List<ASTNode>, src: String, linked: Boolean) {
    var lineStart = false
    for (n in nodes) {
        val t = n.type
        if (t == MarkdownTokenTypes.WHITE_SPACE) {
            // Leading spaces on a continuation line are not significant.
            if (!lineStart) append(src, n.startOffset, n.endOffset)
            continue
        }
        lineStart = false
        when (t) {
            MarkdownTokenTypes.TEXT -> appendUnescaped(src, n.startOffset, n.endOffset)
            MarkdownTokenTypes.EOL -> { append('\n'); lineStart = true }
            // `  \n` hard break: the EOL node that follows supplies the newline.
            MarkdownTokenTypes.HARD_LINE_BREAK -> {}
            // `>` marker of a lazy quote continuation line.
            MarkdownTokenTypes.BLOCK_QUOTE -> lineStart = true
            MarkdownElementTypes.EMPH -> styled(ITALIC_SPAN) { appendInline(withoutMarkers(n.children), src, linked) }
            MarkdownElementTypes.STRONG -> styled(BOLD_SPAN) { appendInline(withoutMarkers(n.children), src, linked) }
            GFMElementTypes.STRIKETHROUGH -> styled(STRIKE_SPAN) { appendInline(withoutMarkers(n.children), src, linked) }
            MarkdownElementTypes.CODE_SPAN -> styled(CODE_SPAN) {
                for (c in n.children) {
                    if (c.type != MarkdownTokenTypes.BACKTICK) append(src, c.startOffset, c.endOffset)
                }
            }
            MarkdownElementTypes.INLINE_LINK -> appendInlineLink(n, src, linked)
            MarkdownElementTypes.IMAGE -> {
                val inner = n.children.firstOrNull { it.type == MarkdownElementTypes.INLINE_LINK }
                if (inner != null) appendInlineLink(inner, src, linked) else append(src, n.startOffset, n.endOffset)
            }
            // <https://example.com>
            MarkdownElementTypes.AUTOLINK -> {
                val url = src.substring(n.startOffset + 1, n.endOffset - 1)
                if (linked) append(url) else link(url) { append(url) }
            }
            // Bare https://example.com or www.example.com
            GFMTokenTypes.GFM_AUTOLINK -> {
                val shown = n.text(src)
                val url = if (shown.startsWith("www.")) "https://$shown" else shown
                if (linked) append(shown) else link(url) { append(shown) }
            }
            // Everything else (HTML tags, `$..$` math, reference links, punctuation
            // tokens, ...) is shown as its source text so nothing silently disappears.
            else -> if (n.children.isEmpty()) append(src, n.startOffset, n.endOffset)
            else appendInline(n.children, src, linked)
        }
    }
}

private fun AnnotatedString.Builder.appendInlineLink(n: ASTNode, src: String, linked: Boolean) {
    val label = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_TEXT }
    val dest = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_DESTINATION }
    val url = dest?.text(src)?.trim()?.removeSurrounding("<", ">").orEmpty()
    val inner = label?.children
        ?.filter { it.type != MarkdownTokenTypes.LBRACKET && it.type != MarkdownTokenTypes.RBRACKET }
        .orEmpty()
    if (linked) {
        appendInline(trimEdges(inner), src, linked = true)
    } else {
        link(url) { appendInline(trimEdges(inner), src, linked = true) }
    }
}

/** Append src[start, end) with CommonMark backslash escapes (`\*`, `\|`, ...) resolved. */
private fun AnnotatedString.Builder.appendUnescaped(src: String, start: Int, end: Int) {
    var runStart = start
    var i = start
    while (i < end) {
        if (src[i] == '\\' && i + 1 < end && isAsciiPunctuation(src[i + 1])) {
            append(src, runStart, i)
            append(src[i + 1])
            i += 2
            runStart = i
        } else {
            i++
        }
    }
    append(src, runStart, end)
}

private fun isAsciiPunctuation(c: Char): Boolean =
    c in '!'..'/' || c in ':'..'@' || c in '['..'`' || c in '{'..'~'
