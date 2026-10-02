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
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.console.mobile.ui.theme.ConsoleColors
import com.swmansion.enriched.markdown.compose.EnrichedMarkdownText
import com.swmansion.enriched.markdown.compose.Md4cFlags
import com.swmansion.enriched.markdown.compose.markdownStyle

private val ConsoleMarkdownStyle = markdownStyle {
    paragraph {
        fontSize = 15.sp
        color = ConsoleColors.TextPrimary
        lineHeight = 22.sp
        marginBottom = 10.dp
    }
    h1 { fontSize = 20.sp; color = ConsoleColors.TextPrimary }
    h2 { fontSize = 18.sp; color = ConsoleColors.TextPrimary }
    h3 { fontSize = 16.sp; color = ConsoleColors.TextPrimary }
    link {
        color = Color(0xFF7DD3FC)
        textDecoration = TextDecoration.Underline
    }
    codeBlock {
        fontSize = 13.sp
        color = ConsoleColors.Syntax.Plain
        backgroundColor = ConsoleColors.Card
        cornerRadius = 10.dp
        padding = 12.dp
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
