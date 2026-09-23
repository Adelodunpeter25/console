package com.console.mobile.core.util

import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import com.console.mobile.ui.theme.ConsoleColors

private val KEYWORDS = setOf(
    "as", "break", "case", "catch", "class", "const", "continue", "data", "do",
    "else", "enum", "export", "extends", "false", "finally", "for", "from", "fun",
    "if", "implements", "import", "in", "interface", "is", "nil", "null", "object",
    "package", "private", "protected", "public", "return", "sealed", "struct",
    "super", "this", "throw", "true", "try", "type", "typeof", "val", "var",
    "when", "while", "yield", "fn", "let", "mut", "use", "mod", "impl", "match",
    "def", "lambda", "with", "pass", "raise", "await", "async", "new", "delete",
    "switch", "default", "do", "static", "final", "void", "int", "float", "double",
    "boolean", "string", "select", "where", "from", "table", "yes", "no", "on", "off",
)

private val TOKEN_REGEX = Regex(
    """"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`|//.*|#.*|/\*.*?\*/|\b\d[\d_]*(?:\.\d+)?\b|\b[A-Za-z_][A-Za-z0-9_.-]*\b"""
)

fun languageForPath(path: String?): String {
    if (path.isNullOrBlank()) return ""
    val name = path.substringAfterLast('/').substringAfterLast('\\')
    val dot = name.lastIndexOf('.')
    if (dot <= 0 || dot == name.length - 1) {
        val lower = name.lowercase()
        return when {
            lower.startsWith("dockerfile") -> "docker"
            lower.startsWith("makefile") -> "make"
            else -> ""
        }
    }
    return name.substring(dot + 1).lowercase()
}

private fun styleForToken(token: String, line: String, matchEnd: Int): SpanStyle {
    val syntax = ConsoleColors.Syntax
    if (token.startsWith("//") || token.startsWith("#") || token.startsWith("/*")) {
        return SpanStyle(color = syntax.Comment)
    }
    if (token.startsWith("\"") || token.startsWith("'") || token.startsWith("`")) {
        return SpanStyle(color = syntax.String)
    }
    if (token.firstOrNull()?.isDigit() == true) {
        return SpanStyle(color = syntax.Number)
    }
    if (KEYWORDS.contains(token)) {
        return SpanStyle(color = syntax.Keyword)
    }
    if (token.firstOrNull()?.isUpperCase() == true) {
        return SpanStyle(color = syntax.ClassName)
    }
    // function / builtin call: next non-space char is '('
    var j = matchEnd
    while (j < line.length && line[j].isWhitespace()) j++
    if (j < line.length && line[j] == '(') {
        return SpanStyle(color = syntax.Builtin)
    }
    return SpanStyle(color = syntax.Plain)
}

/**
 * Single-line regex highlighter using the shared Vitesse-dark palette.
 * Fast enough for capped file previews; keeps diff +/- tint via [base].
 */
fun highlightLine(line: String, language: String = "", base: Color = ConsoleColors.Syntax.Plain): AnnotatedString {
    if (line.isEmpty()) return AnnotatedString("")
    val trimmed = line.trim()
    // TOML/INI section header: [scripts.dev-mobile] -> type color.
    if ((language == "toml" || language == "ini" || language == "cfg") &&
        trimmed.startsWith("[") && trimmed.endsWith("]")
    ) {
        return AnnotatedString(line, SpanStyle(color = ConsoleColors.Syntax.Type, fontWeight = FontWeight.SemiBold))
    }
    return buildAnnotatedString {
        pushStyle(SpanStyle(color = base))
        var cursor = 0
        // TOML/YAML/ENV key before = or : -> builtin blue.
        val keyMatch = Regex("""^(\s*)([A-Za-z0-9_.\-]+)(\s*[:=])""").find(line)
        val keyEnd: Int
        if (keyMatch != null && (language == "toml" || language == "yaml" || language == "yml" || language == "env" || language == "ini" || language == "cfg" || language == "sh" || language == "bash" || language == "zsh")) {
            val prefix = keyMatch.groupValues[1]
            val key = keyMatch.groupValues[2]
            val sep = keyMatch.groupValues[3]
            append(prefix)
            pushStyle(SpanStyle(color = ConsoleColors.Syntax.Builtin))
            append(key)
            pop()
            append(sep)
            cursor = keyMatch.range.last + 1
            keyEnd = cursor
        } else {
            keyEnd = 0
        }
        TOKEN_REGEX.findAll(line, startIndex = cursor).forEach { m ->
            val s = m.range.first
            val e = m.range.last + 1
            if (s > cursor) append(line.substring(cursor, s))
            pushStyle(styleForToken(m.value, line, e))
            append(m.value)
            pop()
            cursor = e
        }
        if (cursor < line.length) append(line.substring(cursor))
        // silence unused warning for future lang-specific branches
        @Suppress("UNUSED_EXPRESSION") keyEnd
        pop()
    }
}
