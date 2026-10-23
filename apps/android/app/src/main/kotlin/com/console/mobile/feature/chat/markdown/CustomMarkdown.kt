package com.console.mobile.feature.chat.markdown

import com.console.mobile.SharedNative
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.InlineTextContent
import androidx.compose.foundation.text.appendInlineContent
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.LaunchedEffect
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
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.LinkAnnotation
import androidx.compose.ui.text.Placeholder
import androidx.compose.ui.text.PlaceholderVerticalAlign
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import coil3.compose.AsyncImage
import com.console.mobile.core.util.FILE_LINK_PREFIX
import com.console.mobile.core.util.PageTitles
import com.console.mobile.core.util.bareWebUrls
import com.console.mobile.core.util.faviconHostOrNull
import com.console.mobile.core.util.fileLinkTarget
import com.console.mobile.core.util.filePathOrNull
import com.console.mobile.ui.components.Favicon
import com.console.mobile.ui.components.FileIcon
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.console.mobile.ui.theme.NewTheme
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

/** Native markdown renderer used by chat, tools, subagents, and the file viewer. */
@Composable
fun CustomMarkdown(content: String, modifier: Modifier = Modifier, streaming: Boolean = false) {
    if (content.isBlank()) return
    SelectionContainer(modifier = modifier) {
        // The parser object survives token updates. Streaming updates only replace
        // the unstable tail when possible; completed blocks retain their model.
        val parser = remember { StreamingMarkdownParser() }
        // A title arriving later must rebuild blocks that were parsed without it.
        val blocks = parser.update(content, streaming, PageTitles.version)
        // Only settled text is looked up, so a URL still streaming in is never fetched.
        LaunchedEffect(content, streaming) {
            if (!streaming) bareWebUrls(content).forEach(PageTitles::request)
        }
        Column(Modifier.fillMaxWidth()) {
            MdBlocks(blocks, NewTheme.TextPrimary, 15.sp, 22.sp, 6.dp)
            if (streaming) Text("▍", color = NewTheme.TextMuted, fontSize = 14.sp)
        }
    }
}

@Composable
private fun MdBlocks(blocks: List<MdBlock>, color: Color, size: TextUnit, lineHeight: TextUnit, gap: Dp) {
    blocks.forEachIndexed { index, block -> key(index) { MdBlockView(block, color, size, lineHeight, gap) } }
}

@Composable
private fun MdBlockView(block: MdBlock, color: Color, size: TextUnit, lineHeight: TextUnit, gap: Dp) {
    when (block) {
        is MdBlock.Code -> CodeBlock(block.language, block.code)
        is MdBlock.Image -> MarkdownImage(block.url, block.alt)
        is MdBlock.Math -> Latex(block.expression, block.display)
        is MdBlock.Heading -> Text(
            block.text,
            inlineContent = rememberInlineContent(block.text),
            color = NewTheme.TextPrimary,
            fontSize = when (block.level) {
                1 -> 22.sp; 2 -> 19.sp; 3 -> 17.sp; 4 -> 16.sp; 5 -> 15.sp; else -> 14.sp
            },
            lineHeight = when (block.level) {
                1 -> 28.sp; 2 -> 25.sp; 3 -> 23.sp; 4 -> 22.sp; 5 -> 21.sp; else -> 20.sp
            },
            fontWeight = if (block.level <= 2) FontWeight.Bold else FontWeight.SemiBold,
            modifier = Modifier.padding(top = 8.dp, bottom = 4.dp),
        )
        is MdBlock.Quote -> Row(
            Modifier.fillMaxWidth().padding(vertical = 6.dp),
            verticalAlignment = Alignment.Top,
        ) {
            Box(Modifier.width(2.dp).fillMaxHeight().background(NewTheme.QuoteBar))
            Column(Modifier.padding(start = 10.dp)) {
                MdBlocks(block.blocks, NewTheme.TextSecondary, 14.sp, TextUnit.Unspecified, 4.dp)
            }
        }
        MdBlock.Rule -> Box(
            Modifier.fillMaxWidth().padding(vertical = 12.dp).background(NewTheme.Divider).height(1.dp),
        )
        is MdBlock.ListBlock -> ListView(block, color)
        is MdBlock.Table -> TableView(block)
        is MdBlock.Paragraph -> MarkdownTextBlock(block.text, color, size, lineHeight, gap)
    }
}

@Composable
private fun MarkdownTextBlock(text: AnnotatedString, color: Color, size: TextUnit, lineHeight: TextUnit, gap: Dp) {
    val inlineContent = rememberInlineContent(text)
    Text(
        text = text,
        color = color,
        fontSize = size,
        lineHeight = lineHeight,
        inlineContent = inlineContent,
        modifier = Modifier.fillMaxWidth().padding(bottom = gap),
    )
}

/** Inline placeholders (latex, file icons, favicons) referenced by [text]. */
@Composable
private fun rememberInlineContent(text: AnnotatedString): Map<String, InlineTextContent> {
    // Sizes need typesetting, which is composable-scoped; collect them first, then build the map once.
    val sizes = text.getStringAnnotations(TAG_LATEX, 0, text.length).associate { it.item to rememberInlineMathSize(it.item) }
    return remember(text, sizes) { buildInlineContent(text, sizes) }
}

private fun buildInlineContent(text: AnnotatedString, sizes: Map<String, InlineMathSize>): Map<String, InlineTextContent> {
    val out = HashMap<String, InlineTextContent>()
    text.getStringAnnotations(TAG_LATEX, 0, text.length).forEach { a ->
        val size = sizes.getValue(a.item)
        out[a.item] = InlineTextContent(
            Placeholder(size.widthSp.sp, size.heightSp.sp, PlaceholderVerticalAlign.Center),
            children = { Latex(a.item, false) },
        )
    }
    text.getStringAnnotations(TAG_ICON, 0, text.length).forEach { a ->
        val value = a.item.substringAfter(':')
        out[a.item] = InlineTextContent(
            Placeholder(21.sp, 16.sp, PlaceholderVerticalAlign.Center),
            children = {
                // Icon at the start of a wider box: the empty tail is the gap before the label.
                Box(Modifier.fillMaxSize(), contentAlignment = Alignment.CenterStart) {
                    if (a.item.startsWith("file:")) FileIcon(filename = value, sizeDp = 14) else Favicon(host = value, sizeDp = 14)
                }
            },
        )
    }
    return out
}

private const val TAG_LATEX = "latex"
private const val TAG_ICON = "icon"

/** Icon placeholder + a space, so the label never touches it. */
private fun AnnotatedString.Builder.appendIcon(kind: String, value: String) {
    val id = "$kind:$value"
    appendInlineContent(id, " ")
    addStringAnnotation(TAG_ICON, id, length - 1, length)
    append(' ')
}

@Composable
private fun MarkdownImage(url: String, alt: String) {
    Column(Modifier.fillMaxWidth().padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        AsyncImage(
            model = url,
            contentDescription = alt.ifBlank { null },
            contentScale = ContentScale.Fit,
            modifier = Modifier.fillMaxWidth().heightIn(max = 360.dp),
        )
        if (alt.isNotBlank()) Text(alt, color = NewTheme.TextMuted, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp))
    }
}

@Composable
private fun ListView(block: MdBlock.ListBlock, color: Color) {
    Column(Modifier.padding(vertical = 2.dp)) {
        block.items.forEachIndexed { i, item ->
            val marker = when {
                item.checked != null -> if (item.checked) "☑" else "☐"
                block.ordered -> "${block.start + i}."
                else -> "•"
            }
            Row(Modifier.padding(vertical = 2.dp)) {
                Text(marker, color = NewTheme.TextSecondary, fontSize = 14.sp, modifier = Modifier.widthIn(min = 20.dp).padding(end = 6.dp))
                Column(Modifier.weight(1f)) { MdBlocks(item.blocks, color, 14.sp, TextUnit.Unspecified, 0.dp) }
            }
        }
    }
}

private enum class TableAlign { LEFT, CENTER, RIGHT }

@Composable
private fun TableView(block: MdBlock.Table) {
    val measurer = rememberTextMeasurer()
    val density = LocalDensity.current
    val style = TextStyle(fontSize = 13.sp)
    val widths = remember(block, measurer, density) {
        val cols = maxOf(block.header.size, block.rows.maxOfOrNull { it.size } ?: 0)
        List(cols) { c ->
            val cells = buildList {
                block.header.getOrNull(c)?.let(::add)
                block.rows.forEach { it.getOrNull(c)?.let(::add) }
            }
            val px = cells.maxOfOrNull { measurer.measure(it, style).size.width } ?: 0
            with(density) { (px.toDp() + 20.dp).coerceIn(72.dp, 360.dp) }
        }
    }
    val shape = RoundedCornerShape(8.dp)
    Column(
        Modifier.padding(vertical = 8.dp).horizontalScroll(rememberScrollState()).clip(shape).border(1.dp, NewTheme.Divider, shape),
    ) {
        TableRowView(block.header, widths, block.align, true)
        block.rows.forEach { TableRowView(it, widths, block.align, false) }
    }
}

@Composable
private fun TableRowView(cells: List<AnnotatedString>, widths: List<Dp>, align: List<TableAlign>, header: Boolean) {
    Row(Modifier.height(IntrinsicSize.Min).background(if (header) NewTheme.Raised else Color.Transparent)) {
        widths.forEachIndexed { c, width ->
            val cell = cells.getOrNull(c) ?: AnnotatedString("")
            Text(
                cell,
                inlineContent = rememberInlineContent(cell),
                color = NewTheme.TextPrimary,
                fontSize = 13.sp,
                fontWeight = if (header) FontWeight.SemiBold else null,
                textAlign = when (align.getOrNull(c)) { TableAlign.CENTER -> TextAlign.Center; TableAlign.RIGHT -> TextAlign.End; else -> TextAlign.Start },
                modifier = Modifier.width(width).fillMaxHeight().border(0.5.dp, NewTheme.Divider).padding(horizontal = 10.dp, vertical = 7.dp),
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
    // Keep code in the parent chat scroll. Sora's vertical scroll inside a
    // LazyColumn made long conversations feel like nested-scroll traps. Plain
    // Compose text grows naturally and only scrolls horizontally for long lines.
    Column(Modifier.fillMaxWidth().padding(vertical = 10.dp)) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
            com.console.mobile.ui.components.FileLanguageIcon(language, 13)
            Text(language.ifBlank { "code" }, color = NewTheme.TextSecondary, fontSize = 11.sp, fontFamily = ConsoleMonoFamily, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f).padding(start = 6.dp))
            IconButton(onClick = {
                clipboard.setText(AnnotatedString(code)); copied = true
                scope.launch { delay(1500); copied = false }
            }) { if (copied) Icon(TablerIcons.Outline.Check, "Copied", tint = NewTheme.Success) else Icon(TablerIcons.Outline.Copy, "Copy code", tint = NewTheme.TextSecondary) }
        }
        Text(
            capped,
            color = NewTheme.TextPrimary,
            fontFamily = ConsoleMonoFamily,
            fontSize = 13.sp,
            lineHeight = 19.sp,
            modifier = Modifier.fillMaxWidth().horizontalScroll(rememberScrollState())
                .background(NewTheme.Output)
                .padding(horizontal = 14.dp, vertical = 10.dp),
        )
    }
}

@Immutable
private sealed interface MdBlock {
    @Immutable data class Code(val language: String, val code: String) : MdBlock
    @Immutable data class Image(val url: String, val alt: String) : MdBlock
    @Immutable data class Math(val expression: String, val display: Boolean) : MdBlock
    @Immutable data class Heading(val level: Int, val text: AnnotatedString) : MdBlock
    @Immutable data class Quote(val blocks: List<MdBlock>) : MdBlock
    data object Rule : MdBlock
    @Immutable data class ListBlock(val ordered: Boolean, val start: Int, val items: List<MdListItem>) : MdBlock
    @Immutable data class Paragraph(val text: AnnotatedString) : MdBlock
    @Immutable data class Table(val header: List<AnnotatedString>, val rows: List<List<AnnotatedString>>, val align: List<TableAlign>) : MdBlock
}

@Immutable private data class MdListItem(val checked: Boolean?, val blocks: List<MdBlock>)

private val PARSER = MarkdownParser(GFMFlavourDescriptor())
private val ATX_LEVELS = mapOf(
    MarkdownElementTypes.ATX_1 to 1, MarkdownElementTypes.ATX_2 to 2, MarkdownElementTypes.ATX_3 to 3,
    MarkdownElementTypes.ATX_4 to 4, MarkdownElementTypes.ATX_5 to 5, MarkdownElementTypes.ATX_6 to 6,
)

/** Avoids reparsing settled blocks on every streamed token. */
private class StreamingMarkdownParser {
    private var source = ""
    private var blocks: List<MdBlock> = emptyList()
    private var renderedStreaming = false

    private var titleVersion = 0

    fun update(next: String, streaming: Boolean, titles: Int): List<MdBlock> {
        if (titles != titleVersion) { titleVersion = titles; source = "" }
        if (next == source && streaming == renderedStreaming) return blocks
        if (next == source && !streaming && renderedStreaming) {
            blocks = parseBlocks(next)
            renderedStreaming = false
            return blocks
        }
        val appendOnly = streaming && next.startsWith(source) && !hasLinkDefinition(next)
        if (!appendOnly || source.isEmpty()) {
            source = next
            blocks = parseBlocks(next)
            renderedStreaming = streaming
            return blocks
        }
        // Markdown block structure can only change in the final block while a
        // response is appended. Preserve the settled prefix and parse its tail.
        val boundary = source.lastIndexOf("\n\n").let { if (it < 0) 0 else it + 2 }
        val prefix = if (boundary == 0) emptyList() else parseBlocks(source.substring(0, boundary))
        source = next
        // Parse a synthetic, closed display tail so emphasis and links do not
        // flicker while their closing characters are still arriving. The
        // canonical source remains untouched and is reparsed when streaming
        // ends or when a non-append update arrives.
        val tailSource = next.substring(boundary)
        val displayTail = if (streaming) SharedNative.mendMarkdown(tailSource) ?: tailSource else tailSource
        val tail = parseBlocks(displayTail)
        blocks = prefix + tail
        renderedStreaming = streaming
        return blocks
    }
}

private fun parseBlocks(src: String): List<MdBlock> {
    val root = PARSER.buildMarkdownTreeFromString(src)
    val refs = root.children.filter { it.type == MarkdownElementTypes.LINK_DEFINITION }.associate { definitionKey(it, src) to definitionUrl(it, src) }
    return blocksOf(root.children, src, refs)
}

private fun ASTNode.text(src: String): String = src.substring(startOffset, endOffset)

private fun blocksOf(nodes: List<ASTNode>, src: String, refs: Map<String, String>): List<MdBlock> {
    val out = ArrayList<MdBlock>(nodes.size)
    for (n in nodes) {
        when (n.type) {
            MarkdownElementTypes.PARAGRAPH -> {
                val math = n.children.firstOrNull { it.type == GFMElementTypes.BLOCK_MATH }
                if (math != null) {
                    out += MdBlock.Math(math.text(src).removeSurrounding("$$").trim(), true)
                } else {
                    val images = n.children.filter { it.type == MarkdownElementTypes.IMAGE }
                    if (images.isEmpty()) {
                        val text = inlineOf(n.children, src, refs)
                        if (text.text.isNotBlank()) out += MdBlock.Paragraph(text)
                    } else {
                        // Split image nodes out so the URL is actually loaded and
                        // displayed instead of being reduced to its alt text.
                        var cursor = n.startOffset
                        for (image in images) {
                            val before = src.substring(cursor, image.startOffset)
                            if (before.isNotBlank()) out += MdBlock.Paragraph(inlineOf(PARSER.buildMarkdownTreeFromString(before).children.flatMap { it.children }, before, refs))
                            imageInfo(image, src, refs)?.let { out += MdBlock.Image(it.first, it.second) }
                            cursor = image.endOffset
                        }
                        val after = src.substring(cursor, n.endOffset)
                        if (after.isNotBlank()) out += MdBlock.Paragraph(inlineOf(PARSER.buildMarkdownTreeFromString(after).children.flatMap { it.children }, after, refs))
                    }
                }
            }
            MarkdownElementTypes.ATX_1, MarkdownElementTypes.ATX_2, MarkdownElementTypes.ATX_3,
            MarkdownElementTypes.ATX_4, MarkdownElementTypes.ATX_5, MarkdownElementTypes.ATX_6 ->
                headingBlock(n, MarkdownTokenTypes.ATX_CONTENT, ATX_LEVELS.getValue(n.type), src, refs)?.let { out += it }
            MarkdownElementTypes.SETEXT_1 -> headingBlock(n, MarkdownTokenTypes.SETEXT_CONTENT, 1, src, refs)?.let { out += it }
            MarkdownElementTypes.SETEXT_2 -> headingBlock(n, MarkdownTokenTypes.SETEXT_CONTENT, 2, src, refs)?.let { out += it }
            MarkdownElementTypes.CODE_FENCE -> out += fenceBlock(n, src)
            MarkdownElementTypes.CODE_BLOCK -> out += indentedCodeBlock(n, src)
            MarkdownElementTypes.BLOCK_QUOTE -> out += MdBlock.Quote(blocksOf(n.children, src, refs))
            MarkdownElementTypes.UNORDERED_LIST, MarkdownElementTypes.ORDERED_LIST -> out += listBlock(n, src, refs)
            MarkdownTokenTypes.HORIZONTAL_RULE -> out += MdBlock.Rule
            GFMElementTypes.TABLE -> out += tableBlock(n, src, refs)
            MarkdownElementTypes.HTML_BLOCK -> {
                val clean = cleanHtml(n.text(src)).trim()
                if (clean.isNotEmpty()) out += MdBlock.Paragraph(AnnotatedString(clean))
            }
        }
    }
    return out
}

private fun headingBlock(n: ASTNode, contentType: IElementType, level: Int, src: String, refs: Map<String, String>): MdBlock? {
    val content = n.children.firstOrNull { it.type == contentType } ?: return null
    return MdBlock.Heading(level.coerceIn(1, 6), inlineOf(content.children, src, refs))
}

private fun fenceBlock(n: ASTNode, src: String): MdBlock.Code {
    var lang = ""; var indent = 0; var openerEolSeen = false; val sb = StringBuilder()
    for (c in n.children) when (c.type) {
        MarkdownTokenTypes.CODE_FENCE_START -> { val t = c.text(src); indent = t.length - t.trimStart().length }
        MarkdownTokenTypes.FENCE_LANG -> lang = c.text(src).trim().substringBefore(' ')
        MarkdownTokenTypes.EOL -> if (openerEolSeen) sb.append('\n') else openerEolSeen = true
        MarkdownTokenTypes.CODE_FENCE_CONTENT -> sb.append(stripIndent(c.text(src), indent))
    }
    return MdBlock.Code(lang, sb.toString())
}

private fun indentedCodeBlock(n: ASTNode, src: String): MdBlock.Code = MdBlock.Code("", buildString {
    n.children.forEach { when (it.type) { MarkdownTokenTypes.CODE_LINE -> append(stripIndent(it.text(src), 4)); MarkdownTokenTypes.EOL -> append('\n') } }
})
private fun stripIndent(line: String, n: Int): String {
    var count = 0
    while (count < n && count < line.length && line[count] == ' ') count++
    return line.substring(count)
}

private fun listBlock(n: ASTNode, src: String, refs: Map<String, String>): MdBlock.ListBlock {
    val ordered = n.type == MarkdownElementTypes.ORDERED_LIST; var start = 1; val items = ArrayList<MdListItem>()
    for (item in n.children) {
        if (item.type != MarkdownElementTypes.LIST_ITEM) continue
        if (ordered && items.isEmpty()) start = item.children.firstOrNull { it.type == MarkdownTokenTypes.LIST_NUMBER }?.text(src)?.filter(Char::isDigit)?.toIntOrNull() ?: 1
        val box = item.children.firstOrNull { it.type == GFMTokenTypes.CHECK_BOX }
        items += MdListItem(box?.text(src)?.contains('x', true), blocksOf(item.children, src, refs))
    }
    return MdBlock.ListBlock(ordered, start, items)
}

private fun tableBlock(n: ASTNode, src: String, refs: Map<String, String>): MdBlock.Table {
    var header = emptyList<AnnotatedString>(); val rows = ArrayList<List<AnnotatedString>>(); var align = emptyList<TableAlign>()
    for (c in n.children) when (c.type) {
        GFMElementTypes.HEADER -> {
            header = tableCells(c, src, refs)
            align = c.children.firstOrNull { it.type == GFMTokenTypes.TABLE_SEPARATOR }
                ?.text(src)?.let(::parseAlignments).orEmpty()
        }
        GFMElementTypes.ROW -> rows += tableCells(c, src, refs)
    }
    return MdBlock.Table(header, rows, align)
}
private fun tableCells(row: ASTNode, src: String, refs: Map<String, String>): List<AnnotatedString> = row.children.filter { it.type == GFMTokenTypes.CELL }.map { inlineOf(it.children, src, refs) }
private fun parseAlignments(separator: String): List<TableAlign> = separator.trim().trim('|').split('|').map {
    val s = it.trim(); when { s.startsWith(":") && s.endsWith(":") -> TableAlign.CENTER; s.endsWith(":") -> TableAlign.RIGHT; else -> TableAlign.LEFT }
}

private val CODE_SPAN = SpanStyle(color = NewTheme.CodeText, fontFamily = ConsoleMonoFamily, fontSize = 13.sp, background = NewTheme.CodeWash)
private val BOLD_SPAN = SpanStyle(fontWeight = FontWeight.Bold, color = NewTheme.TextPrimary)
private val STRIKE_SPAN = SpanStyle(textDecoration = TextDecoration.LineThrough)
private val ITALIC_SPAN = SpanStyle(fontStyle = FontStyle.Italic)
private val LINK_STYLES = TextLinkStyles(style = SpanStyle(color = NewTheme.Link))

private fun inlineOf(nodes: List<ASTNode>, src: String, refs: Map<String, String>): AnnotatedString = buildAnnotatedString { appendInline(trimEdges(nodes), src, refs, false) }
private fun trimEdges(nodes: List<ASTNode>): List<ASTNode> {
    fun blank(node: ASTNode) = node.type == MarkdownTokenTypes.WHITE_SPACE || node.type == MarkdownTokenTypes.EOL
    var start = 0
    var end = nodes.size
    while (start < end && blank(nodes[start])) start++
    while (end > start && blank(nodes[end - 1])) end--
    return nodes.subList(start, end)
}
private inline fun AnnotatedString.Builder.styled(style: SpanStyle, block: () -> Unit) { pushStyle(style); block(); pop() }
private inline fun AnnotatedString.Builder.link(url: String, block: () -> Unit) {
    if (url.isBlank()) { block(); return }
    // File paths get their file-type icon and open in the Files tab; web
    // links get the site favicon.
    val host = faviconHostOrNull(url)
    val path = if (host == null) fileLinkTarget(url) else null
    when {
        host != null -> appendIcon("favicon", host)
        path != null -> appendIcon("file", path)
    }
    pushLink(LinkAnnotation.Url(if (path != null) FILE_LINK_PREFIX + path else url, LINK_STYLES)); block(); pop()
}
private fun withoutMarkers(nodes: List<ASTNode>): List<ASTNode> = nodes.filter { it.type != MarkdownTokenTypes.EMPH && it.type != GFMTokenTypes.TILDE }

private fun AnnotatedString.Builder.appendInline(nodes: List<ASTNode>, src: String, refs: Map<String, String>, linked: Boolean) {
    for (n in nodes) when (n.type) {
        MarkdownTokenTypes.WHITE_SPACE -> append(src, n.startOffset, n.endOffset)
        MarkdownTokenTypes.EOL -> append('\n')
        MarkdownTokenTypes.TEXT -> appendUnescaped(src, n.startOffset, n.endOffset)
        MarkdownTokenTypes.HARD_LINE_BREAK -> Unit
        MarkdownTokenTypes.BLOCK_QUOTE -> Unit
        MarkdownElementTypes.EMPH -> styled(ITALIC_SPAN) { appendInline(withoutMarkers(n.children), src, refs, linked) }
        MarkdownElementTypes.STRONG -> styled(BOLD_SPAN) { appendInline(withoutMarkers(n.children), src, refs, linked) }
        GFMElementTypes.STRIKETHROUGH -> styled(STRIKE_SPAN) { appendInline(withoutMarkers(n.children), src, refs, linked) }
        MarkdownElementTypes.CODE_SPAN -> {
            val code = n.children.filter { it.type != MarkdownTokenTypes.BACKTICK }
            val path = if (linked) null else filePathOrNull(code.joinToString("") { it.text(src) })
            if (path == null) {
                styled(CODE_SPAN) { code.forEach { append(src, it.startOffset, it.endOffset) } }
            } else {
                // A path in backticks: icon, then a tappable code chip that opens the file.
                appendIcon("file", path)
                pushLink(LinkAnnotation.Url(FILE_LINK_PREFIX + path, LINK_STYLES))
                styled(CODE_SPAN) { code.forEach { append(src, it.startOffset, it.endOffset) } }
                pop()
            }
        }
        MarkdownElementTypes.INLINE_LINK -> appendInlineLink(this, n, src, refs, linked)
        MarkdownElementTypes.FULL_REFERENCE_LINK, MarkdownElementTypes.SHORT_REFERENCE_LINK -> appendReferenceLink(this, n, src, refs, linked)
        MarkdownElementTypes.IMAGE -> imageInfo(n, src, refs)?.second?.let(::append)
        MarkdownElementTypes.AUTOLINK -> { val url = src.substring(n.startOffset + 1, n.endOffset - 1); if (linked) append(url) else link(url) { append(PageTitles.get(url) ?: url) } }
        GFMTokenTypes.GFM_AUTOLINK -> { val shown = n.text(src); val target = if (shown.startsWith("www.")) "https://$shown" else shown; link(target) { append(PageTitles.get(target) ?: shown) } }
        GFMElementTypes.INLINE_MATH -> {
            val expression = n.text(src).removeSurrounding("$").trim()
            appendInlineContent(expression, " ")
            addStringAnnotation(TAG_LATEX, expression, length - 1, length)
        }
        else -> if (n.children.isEmpty()) append(formatMath(cleanHtml(n.text(src)))) else appendInline(n.children, src, refs, linked)
    }
}

private fun appendReferenceLink(builder: AnnotatedString.Builder, n: ASTNode, src: String, refs: Map<String, String>, linked: Boolean) {
    val label = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_TEXT } ?: n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_LABEL }
    val keyNode = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_LABEL }
    val key = keyNode?.text(src)?.trim()?.removeSurrounding("[")?.removeSuffix("]")?.ifBlank { label?.text(src)?.trim()?.removeSurrounding("[")?.removeSuffix("]") }?.lowercase().orEmpty()
    val labelNodes = label?.children?.filterNot { it.type == MarkdownTokenTypes.LBRACKET || it.type == MarkdownTokenTypes.RBRACKET }.orEmpty()
    val url = refs[key].orEmpty()
    if (linked) builder.appendInline(labelNodes, src, refs, true) else builder.link(url) { builder.appendInline(labelNodes, src, refs, true) }
}

private fun appendInlineLink(builder: AnnotatedString.Builder, n: ASTNode, src: String, refs: Map<String, String>, linked: Boolean) {
    val label = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_TEXT }
    val dest = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_DESTINATION }
    val url = dest?.text(src)?.trim()?.removeSurrounding("<", ">") ?: ""
    val inner = label?.children?.filterNot { it.type == MarkdownTokenTypes.LBRACKET || it.type == MarkdownTokenTypes.RBRACKET }.orEmpty()
    if (linked) builder.appendInline(inner, src, refs, true) else builder.link(url) { builder.appendInline(inner, src, refs, true) }
}

private fun imageInfo(n: ASTNode, src: String, refs: Map<String, String>): Pair<String, String>? {
    val inner = n.children.firstOrNull { it.type == MarkdownElementTypes.INLINE_LINK || it.type == MarkdownElementTypes.FULL_REFERENCE_LINK } ?: n
    val label = inner.children.firstOrNull { it.type == MarkdownElementTypes.LINK_TEXT }
    val alt = label?.text(src)?.removeSurrounding("[")?.removeSuffix("]").orEmpty()
    val dest = inner.children.firstOrNull { it.type == MarkdownElementTypes.LINK_DESTINATION }?.text(src)?.trim()?.removeSurrounding("<", ">")
    if (dest != null) return dest to alt
    val key = inner.children.firstOrNull { it.type == MarkdownElementTypes.LINK_LABEL }?.text(src)?.trim()?.removeSurrounding("[")?.removeSuffix("]")?.lowercase()
    return key?.let { refs[it] }?.let { it to alt }
}

private fun definitionKey(n: ASTNode, src: String): String = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_LABEL }?.text(src)?.trim()?.removeSurrounding("[")?.removeSuffix("]")?.lowercase().orEmpty()
private fun definitionUrl(n: ASTNode, src: String): String = n.children.firstOrNull { it.type == MarkdownElementTypes.LINK_DESTINATION }?.text(src)?.trim()?.removeSurrounding("<", ">") ?: ""

private fun cleanHtml(text: String): String = text
    .replace(Regex("(?i)<br\\s*/?>"), "\n")
    .replace(Regex("(?s)<[^>]*>"), "")
    .replace("&nbsp;", " ").replace("&amp;", "&").replace("&lt;", "<").replace("&gt;", ">")
    .replace("&quot;", "\"").replace("&#39;", "'")

private fun formatMath(text: String): String {
    var out = text
    listOf("\\rightarrow" to "→", "\\leftarrow" to "←", "\\Rightarrow" to "⇒", "\\Leftarrow" to "⇐", "\\leftrightarrow" to "↔", "\\to" to "→", "\\leq" to "≤", "\\geq" to "≥", "\\neq" to "≠", "\\approx" to "≈", "\\times" to "×", "\\cdot" to "·", "\\pm" to "±", "\\dots" to "…", "\\infty" to "∞").forEach { (from, to) -> out = out.replace(from, to) }
    return out
}
private fun AnnotatedString.Builder.appendUnescaped(src: String, start: Int, end: Int) {
    var run = start; var i = start
    while (i < end) { if (src[i] == '\\' && i + 1 < end && isAsciiPunctuation(src[i + 1])) { append(src, run, i); append(src[i + 1]); i += 2; run = i } else i++ }
    append(src, run, end)
}
private fun isAsciiPunctuation(c: Char): Boolean = c in '!'..'/' || c in ':'..'@' || c in '['..'`' || c in '{'..'~'
private fun hasLinkDefinition(text: String): Boolean = text.lines().any { it.trimStart().startsWith("[") && it.contains("]:") }
