package com.console.mobile.ui.components

import android.graphics.Typeface
import android.os.Bundle
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.viewinterop.AndroidView
import com.console.mobile.core.util.CODE_KEYWORDS
import com.console.mobile.core.util.CODE_TOKEN_REGEX
import com.console.mobile.ui.theme.ConsoleColors
import io.github.rosemoe.sora.lang.EmptyLanguage
import io.github.rosemoe.sora.lang.Language
import io.github.rosemoe.sora.lang.analysis.AnalyzeManager
import io.github.rosemoe.sora.lang.analysis.SimpleAnalyzeManager
import io.github.rosemoe.sora.lang.completion.CompletionCancelledException
import io.github.rosemoe.sora.lang.completion.CompletionPublisher
import io.github.rosemoe.sora.lang.format.Formatter
import io.github.rosemoe.sora.lang.smartEnter.NewlineHandler
import io.github.rosemoe.sora.lang.styling.MappedSpans
import io.github.rosemoe.sora.lang.styling.Styles
import io.github.rosemoe.sora.lang.styling.TextStyle
import io.github.rosemoe.sora.lang.styling.color.ConstColor
import io.github.rosemoe.sora.lang.styling.line.LineBackground
import io.github.rosemoe.sora.text.CharPosition
import io.github.rosemoe.sora.text.ContentReference
import io.github.rosemoe.sora.widget.CodeEditor
import io.github.rosemoe.sora.widget.SymbolPairMatch
import io.github.rosemoe.sora.widget.schemes.EditorColorScheme

/**
 * Centralized read-only code viewer (Sora Editor).
 * Virtualized — safe for 2k+ line files. Transparent background so code
 * sits directly on the screen (no boxed card). Used by the file viewer,
 * chat code blocks and the diff viewer.
 */
class ConsoleColorScheme : EditorColorScheme() {
    init {
        applyDefault()
        val s = ConsoleColors.Syntax
        setColor(WHOLE_BACKGROUND, 0x00000000)
        setColor(TEXT_NORMAL, s.Plain.toArgb())
        setColor(COMMENT, s.Comment.toArgb())
        setColor(KEYWORD, s.Keyword.toArgb())
        setColor(LITERAL, s.String.toArgb())
        setColor(OPERATOR, s.Number.toArgb())
        setColor(FUNCTION_NAME, s.Builtin.toArgb())
        setColor(IDENTIFIER_NAME, s.ClassName.toArgb())
        setColor(IDENTIFIER_VAR, s.Plain.toArgb())
        setColor(ANNOTATION, s.Number.toArgb())
        setColor(LINE_NUMBER, ConsoleColors.TextMuted.copy(alpha = 0.6f).toArgb())
        // Opaque gutter so horizontally-scrolled code slides behind it instead
        // of bleeding through the pinned line numbers. LINE_NUMBER_PANEL is
        // what 0.21.1 paints for the pinned gutter; BACKGROUND covers the rest.
        // Both match the screen so it still looks uniform.
        setColor(LINE_NUMBER_BACKGROUND, ConsoleColors.Background.toArgb())
        setColor(LINE_NUMBER_PANEL, ConsoleColors.Background.toArgb())
        setColor(LINE_NUMBER_PANEL_TEXT, ConsoleColors.TextMuted.copy(alpha = 0.6f).toArgb())
        setColor(LINE_NUMBER_CURRENT, ConsoleColors.TextSecondary.toArgb())
        setColor(LINE_DIVIDER, ConsoleColors.BorderSubtle.toArgb())
        setColor(CURRENT_LINE, 0x00000000)
        setColor(SELECTED_TEXT_BACKGROUND, ConsoleColors.TextSecondary.copy(alpha = 0.25f).toArgb())
        setColor(SCROLL_BAR_THUMB, ConsoleColors.TextMuted.copy(alpha = 0.4f).toArgb())
        setColor(SCROLL_BAR_TRACK, 0x00000000)
        setColor(BLOCK_LINE, ConsoleColors.BorderSubtle.toArgb())
        setColor(BLOCK_LINE_CURRENT, ConsoleColors.Border.toArgb())
    }

    override fun isDark(): Boolean = true
}

private val KEY_LINE_REGEX = Regex("""^(\s*)([A-Za-z0-9_.\-]+)(\s*[:=])""")
private val KEY_LANGUAGES = setOf("toml", "yaml", "yml", "env", "ini", "cfg", "sh", "bash", "zsh")

private fun soraColorId(token: String, line: String, matchEnd: Int): Int {
    if (token.startsWith("//") || token.startsWith("#") || token.startsWith("/*")) {
        return EditorColorScheme.COMMENT
    }
    if (token.startsWith("\"") || token.startsWith("'") || token.startsWith("`")) {
        return EditorColorScheme.LITERAL
    }
    if (token.firstOrNull()?.isDigit() == true) {
        return EditorColorScheme.OPERATOR
    }
    if (CODE_KEYWORDS.contains(token)) {
        return EditorColorScheme.KEYWORD
    }
    if (token.firstOrNull()?.isUpperCase() == true) {
        return EditorColorScheme.IDENTIFIER_NAME
    }
    var j = matchEnd
    while (j < line.length && line[j].isWhitespace()) j++
    if (j < line.length && line[j] == '(') {
        return EditorColorScheme.FUNCTION_NAME
    }
    return EditorColorScheme.TEXT_NORMAL
}

class ConsoleAnalyzeManager(
    private val language: String,
    private val addLines: Set<Int>,
    private val removeLines: Set<Int>,
) : SimpleAnalyzeManager<Any?>() {
    override fun analyze(text: StringBuilder, delegate: Delegate<Any?>): Styles {
        val builder = MappedSpans.Builder()
        val raw = text.toString()
        val lines = raw.split("\n")
        lines.forEachIndexed { index, line ->
            if (index % 200 == 0 && delegate.isCancelled) return@forEachIndexed
            // TOML/INI section header: whole line in type color.
            if ((language == "toml" || language == "ini" || language == "cfg") &&
                line.trim().startsWith("[") && line.trim().endsWith("]")
            ) {
                builder.addIfNeeded(index, 0, TextStyle.makeStyle(EditorColorScheme.IDENTIFIER_NAME))
                return@forEachIndexed
            }
            var cursor = 0
            if (language in KEY_LANGUAGES) {
                val keyMatch = KEY_LINE_REGEX.find(line)
                if (keyMatch != null) {
                    val keyStart = keyMatch.groupValues[1].length
                    builder.addIfNeeded(index, keyStart, TextStyle.makeStyle(EditorColorScheme.FUNCTION_NAME))
                    builder.addIfNeeded(index, keyStart + keyMatch.groupValues[2].length, TextStyle.makeStyle(EditorColorScheme.TEXT_NORMAL))
                    cursor = keyMatch.range.last + 1
                }
            }
            CODE_TOKEN_REGEX.findAll(line, startIndex = cursor).forEach { m ->
                val id = soraColorId(m.value, line, m.range.last + 1)
                if (id != EditorColorScheme.TEXT_NORMAL) {
                    builder.addIfNeeded(index, m.range.first, TextStyle.makeStyle(id))
                }
            }
        }
        builder.determine(lines.size)
        builder.addNormalIfNull()
        val styles = Styles(builder.build())
        addLines.forEach { styles.addLineStyle(LineBackground(it, ConstColor(0x1A34D399))) }
        removeLines.forEach { styles.addLineStyle(LineBackground(it, ConstColor(0x1AF87171))) }
        return styles
    }
}

class ConsoleLanguage(
    private val language: String = "",
    private val addLines: Set<Int> = emptySet(),
    private val removeLines: Set<Int> = emptySet(),
) : Language {
    private val manager = ConsoleAnalyzeManager(language, addLines, removeLines)
    private val emptyFallback = EmptyLanguage()

    override fun getAnalyzeManager(): AnalyzeManager = manager
    override fun getInterruptionLevel(): Int = Language.INTERRUPTION_LEVEL_NONE
    @Throws(CompletionCancelledException::class)
    override fun requireAutoComplete(
        content: ContentReference,
        position: CharPosition,
        publisher: CompletionPublisher,
        extraArguments: Bundle,
    ) {
    }
    override fun getIndentAdvance(content: ContentReference, line: Int, column: Int): Int = 0
    override fun useTab(): Boolean = false
    override fun getFormatter(): Formatter = emptyFallback.formatter
    override fun getSymbolPairs(): SymbolPairMatch = EmptyLanguage.EMPTY_SYMBOL_PAIRS
    override fun getNewlineHandlers(): Array<NewlineHandler> = emptyArray()
    override fun destroy() {
        manager.destroy()
    }
}

@Composable
fun CodeViewer(
    code: String,
    language: String = "",
    modifier: Modifier = Modifier,
    showLineNumbers: Boolean = true,
    fontSizeSp: Float = 11f,
    addLines: Set<Int> = emptySet(),
    removeLines: Set<Int> = emptySet(),
) {
    val colorScheme = remember { ConsoleColorScheme() }
    val soraLanguage = remember(language, addLines, removeLines) {
        ConsoleLanguage(language, addLines, removeLines)
    }
    AndroidView(
        modifier = modifier,
        factory = { ctx ->
            CodeEditor(ctx).apply {
                setColorScheme(colorScheme)
                setEditorLanguage(soraLanguage)
                typefaceText = Typeface.MONOSPACE
                typefaceLineNumber = Typeface.MONOSPACE
                setTextSize(fontSizeSp)
                isLineNumberEnabled = showLineNumbers
                isEditable = false
                isHighlightCurrentLine = false
                isHighlightCurrentBlock = false
                isHighlightBracketPair = false
                isCursorAnimationEnabled = false
                isWordwrap = false
                setPinLineNumber(true)
                setScrollBarEnabled(true)
                tabWidth = 4
                setText(code)
            }
        },
        update = { editor ->
            if (editor.editorLanguage !== soraLanguage) editor.setEditorLanguage(soraLanguage)
            if (editor.isLineNumberEnabled != showLineNumbers) editor.isLineNumberEnabled = showLineNumbers
            if (editor.text.toString() != code) editor.setText(code)
        },
        onRelease = { it.release() },
    )
}
