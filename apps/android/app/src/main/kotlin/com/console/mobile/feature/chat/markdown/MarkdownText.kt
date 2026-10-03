package com.console.mobile.feature.chat.markdown

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextLinkStyles
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.mikepenz.markdown.compose.components.markdownComponents
import com.mikepenz.markdown.compose.elements.MarkdownHighlightedCodeBlock
import com.mikepenz.markdown.compose.elements.MarkdownHighlightedCodeFence
import com.mikepenz.markdown.m3.Markdown
import com.mikepenz.markdown.m3.markdownColor
import com.mikepenz.markdown.m3.markdownTypography
import com.mikepenz.markdown.model.markdownAnimations
import com.mikepenz.markdown.model.markdownDimens
import com.mikepenz.markdown.model.markdownPadding
import com.mikepenz.markdown.model.rememberMarkdownState
import dev.snipme.highlights.Highlights
import dev.snipme.highlights.model.SyntaxTheme

private val BodyStyle = TextStyle(
    fontSize = 15.sp,
    lineHeight = 23.sp,
    color = ConsoleColors.TextPrimary,
)

private val CodeStyle = TextStyle(
    fontFamily = ConsoleMonoFamily,
    fontSize = 13.sp,
    lineHeight = 19.sp,
    color = ConsoleColors.Syntax.Plain,
)

/**
 * Code-block palette built from the app's shared syntax colours, so code in
 * chat matches the file viewer. Highlights takes packed 0xRRGGBB ints.
 */
private val ConsoleSyntaxTheme = SyntaxTheme(
    key = "console-dark",
    code = ConsoleColors.Syntax.Plain.rgb(),
    keyword = ConsoleColors.Syntax.Keyword.rgb(),
    string = ConsoleColors.Syntax.String.rgb(),
    literal = ConsoleColors.Syntax.Number.rgb(),
    comment = ConsoleColors.Syntax.Comment.rgb(),
    metadata = ConsoleColors.Syntax.Builtin.rgb(),
    multilineComment = ConsoleColors.Syntax.Comment.rgb(),
    punctuation = ConsoleColors.TextSecondary.rgb(),
    mark = ConsoleColors.Syntax.Type.rgb(),
)

private fun Color.rgb(): Int = toArgb() and 0xFFFFFF

/**
 * Markdown renderer backed by mikepenz/multiplatform-markdown-renderer
 * (native Compose UI, CommonMark + GFM: tables, task lists, alerts, code).
 *
 * Parsing runs off the main thread via [rememberMarkdownState]; `retainState`
 * keeps the last rendered tree on screen while a newer string (e.g. the next
 * streamed chunk) is parsed, so streaming never flashes an empty bubble.
 */
@Composable
fun MarkdownText(content: String, modifier: Modifier = Modifier, streaming: Boolean = false) {
    if (content.isBlank()) return
    val state = rememberMarkdownState(content, retainState = true)
    val highlights = remember { Highlights.Builder().theme(ConsoleSyntaxTheme) }
    val components = remember(highlights) {
        markdownComponents(
            codeFence = {
                MarkdownHighlightedCodeFence(content = it.content, node = it.node, style = it.typography.code, highlightsBuilder = highlights, showHeader = true)
            },
            codeBlock = {
                MarkdownHighlightedCodeBlock(content = it.content, node = it.node, style = it.typography.code, highlightsBuilder = highlights, showHeader = true)
            },
        )
    }
    Column(modifier = modifier.fillMaxWidth()) {
        Markdown(
            markdownState = state,
            modifier = Modifier.fillMaxWidth(),
            components = components,
            colors = markdownColor(
                text = ConsoleColors.TextPrimary,
                codeBackground = ConsoleColors.Card,
                inlineCodeBackground = Color.White.copy(alpha = 0.08f),
                dividerColor = ConsoleColors.Border,
                tableBackground = ConsoleColors.Card,
            ),
            typography = markdownTypography(
                h1 = BodyStyle.copy(fontSize = 22.sp, lineHeight = 28.sp, fontWeight = FontWeight.Bold),
                h2 = BodyStyle.copy(fontSize = 19.sp, lineHeight = 25.sp, fontWeight = FontWeight.Bold),
                h3 = BodyStyle.copy(fontSize = 17.sp, lineHeight = 23.sp, fontWeight = FontWeight.SemiBold),
                h4 = BodyStyle.copy(fontWeight = FontWeight.SemiBold),
                h5 = BodyStyle.copy(fontWeight = FontWeight.SemiBold),
                h6 = BodyStyle.copy(fontWeight = FontWeight.SemiBold),
                text = BodyStyle,
                paragraph = BodyStyle,
                ordered = BodyStyle,
                bullet = BodyStyle,
                list = BodyStyle,
                code = CodeStyle,
                inlineCode = CodeStyle.copy(color = Color(0xFFFDBA74), lineHeight = 23.sp),
                quote = BodyStyle.copy(color = ConsoleColors.TextSecondary),
                table = BodyStyle.copy(fontSize = 13.sp, lineHeight = 19.sp),
                textLink = TextLinkStyles(
                    style = SpanStyle(color = Color(0xFF7DD3FC), textDecoration = TextDecoration.Underline),
                ),
            ),
            padding = markdownPadding(
                block = 5.dp,
                codeBlock = PaddingValues(12.dp),
                blockQuote = PaddingValues(horizontal = 10.dp),
            ),
            dimens = markdownDimens(
                codeBackgroundCornerSize = 10.dp,
                blockQuoteThickness = 3.dp,
                tableCellPadding = 10.dp,
                tableCornerSize = 8.dp,
            ),
            // Size animations fight the chat's stick-to-bottom scroll while a
            // reply streams in, so render size changes immediately.
            animations = markdownAnimations(animateTextSize = { this }),
        )
        if (streaming) {
            Text("▍", color = ConsoleColors.TextMuted, fontSize = 14.sp)
        }
    }
}
