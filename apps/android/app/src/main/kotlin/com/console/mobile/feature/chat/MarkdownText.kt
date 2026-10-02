package com.console.mobile.feature.chat

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors
import com.console.mobile.ui.theme.ConsoleMonoFamily
import com.swmansion.enriched.markdown.compose.EnrichedMarkdownText
import com.swmansion.enriched.markdown.compose.Md4cFlags
import com.swmansion.enriched.markdown.compose.markdownStyle

private val ConsoleMarkdownStyle = markdownStyle {
    paragraph {
        fontSize = 15.sp
        color = ConsoleColors.TextPrimary
        lineHeight = 23.sp
        marginTop = 0.dp
        marginBottom = 10.dp
    }
    h1 { fontSize = 22.sp; fontWeight = FontWeight.Bold; color = ConsoleColors.TextPrimary; marginTop = 14.dp; marginBottom = 8.dp }
    h2 { fontSize = 19.sp; fontWeight = FontWeight.Bold; color = ConsoleColors.TextPrimary; marginTop = 14.dp; marginBottom = 6.dp }
    h3 { fontSize = 17.sp; fontWeight = FontWeight.SemiBold; color = ConsoleColors.TextPrimary; marginTop = 12.dp; marginBottom = 6.dp }
    h4 { fontSize = 15.sp; fontWeight = FontWeight.SemiBold; color = ConsoleColors.TextPrimary; marginTop = 10.dp; marginBottom = 4.dp }
    strong { fontWeight = FontWeight.Bold; color = ConsoleColors.TextPrimary }
    link {
        color = Color(0xFF7DD3FC)
        textDecoration = TextDecoration.Underline
    }
    code {
        fontFamily = ConsoleMonoFamily
        fontSize = 13.sp
        color = Color(0xFFFDBA74)
        backgroundColor = Color.White.copy(alpha = 0.08f)
        borderColor = Color.Transparent
    }
    codeBlock {
        fontFamily = ConsoleMonoFamily
        fontSize = 13.sp
        lineHeight = 19.sp
        color = ConsoleColors.Syntax.Plain
        backgroundColor = ConsoleColors.Card
        borderColor = ConsoleColors.Border
        borderWidth = 1.dp
        cornerRadius = 10.dp
        padding = 12.dp
        marginTop = 4.dp
        marginBottom = 12.dp
    }
    blockquote {
        color = ConsoleColors.TextSecondary
        borderColor = ConsoleColors.Border
        borderWidth = 3.dp
        gapWidth = 10.dp
        marginBottom = 10.dp
    }
    list {
        fontSize = 15.sp
        color = ConsoleColors.TextPrimary
        lineHeight = 23.sp
        bulletColor = ConsoleColors.TextSecondary
        markerColor = ConsoleColors.TextSecondary
        marginBottom = 8.dp
    }
    thematicBreak {
        color = ConsoleColors.Border
        height = 1.dp
        marginTop = 12.dp
        marginBottom = 12.dp
    }
    table {
        fontSize = 13.sp
        color = ConsoleColors.TextPrimary
        lineHeight = 19.sp
        headerBackgroundColor = ConsoleColors.SurfaceElevated
        headerTextColor = ConsoleColors.TextPrimary
        rowEvenBackgroundColor = ConsoleColors.Card
        rowOddBackgroundColor = ConsoleColors.CardAlt
        borderColor = ConsoleColors.Border
        borderWidth = 1.dp
        cornerRadius = 8.dp
        cellPaddingHorizontal = 10.dp
        cellPaddingVertical = 8.dp
        marginBottom = 12.dp
    }
}

/**
 * Markdown renderer backed by Software Mansion's Enriched Markdown
 * (native text, CommonMark + GFM: tables, task lists, nested lists, code blocks).
 */
@Composable
fun MarkdownText(content: String, modifier: Modifier = Modifier, streaming: Boolean = false) {
    if (content.isBlank()) return
    val context = LocalContext.current
    val flags = remember { Md4cFlags(admonitions = true) }
    Column(modifier = modifier.fillMaxWidth()) {
        EnrichedMarkdownText(
            markdown = content,
            modifier = Modifier.fillMaxWidth(),
            style = ConsoleMarkdownStyle,
            flags = flags,
            onLinkClick = { url ->
                runCatching {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
                }
            },
        )
        if (streaming) {
            Text("▍", color = ConsoleColors.TextMuted, fontSize = 14.sp)
        }
    }
}
