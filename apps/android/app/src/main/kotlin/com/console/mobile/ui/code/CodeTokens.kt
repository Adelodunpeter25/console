package com.console.mobile.ui.code

import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import com.console.mobile.ui.theme.ConsoleColors

/**
 * The token vocabulary and per-language rules the code viewer highlights with.
 * This is the only highlighter in the app — it runs inside Sora's
 * `AnalyzeManager` (see CodeViewer.kt), which runs the analysis on a background
 * thread, so the regex work never blocks composition.
 */

/** Identifiers coloured as keywords. A single union across languages: the
 * analyser is deliberately language-agnostic, so `class` in CSS and `class` in
 * Kotlin both highlight rather than maintaining per-language keyword tables. */
val CODE_KEYWORDS = setOf(
    "as", "break", "case", "catch", "class", "const", "continue", "data", "do",
    "else", "enum", "export", "extends", "false", "finally", "for", "from", "fun",
    "if", "implements", "import", "in", "interface", "is", "nil", "null", "object",
    "package", "private", "protected", "public", "return", "sealed", "struct",
    "super", "this", "throw", "true", "try", "type", "typeof", "val", "var",
    "when", "while", "yield", "fn", "let", "mut", "use", "mod", "impl", "match",
    "def", "lambda", "with", "pass", "raise", "await", "async", "new", "delete",
    "switch", "default", "static", "final", "void", "int", "float", "double",
    "boolean", "string", "select", "where", "from", "table", "yes", "no", "on", "off",
)

/** Strings, comments and numbers in one pass, so the analyser needs a single
 * scan per line rather than a per-category pass. */
val CODE_TOKEN_REGEX = Regex(
    """"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`|//.*|#.*|/\*.*?\*/|\b\d[\d_]*(?:\.\d+)?\b|\b[A-Za-z_][A-Za-z0-9_.-]*\b"""
)

/** `key = value` / `key: value` at the head of a line. */
val KEY_LINE_REGEX = Regex("""^(\s*)([A-Za-z0-9_.\-]+)(\s*[:=])""")

/** Languages whose head-of-line key gets the builtins colour (config-ish). */
val KEY_LANGUAGES = setOf("toml", "yaml", "yml", "env", "ini", "cfg", "sh", "bash", "zsh")

/** Languages with `[section]` headers. */
val SECTION_LANGUAGES = setOf("toml", "ini", "cfg")

/**
 * Path -> the short language tag the analyser keys its rules off. The tag is
 * the file extension, with the two common extensionless filenames resolved.
 */
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

private val KEYWORD_STYLE = SpanStyle(color = ConsoleColors.Syntax.Keyword)
private val STRING_STYLE = SpanStyle(color = ConsoleColors.Syntax.String)
private val NUMBER_STYLE = SpanStyle(color = ConsoleColors.Syntax.Number)
private val COMMENT_STYLE = SpanStyle(color = ConsoleColors.Syntax.Comment)
private val BUILTIN_STYLE = SpanStyle(color = ConsoleColors.Syntax.Builtin)
private val CLASS_NAME_STYLE = SpanStyle(color = ConsoleColors.Syntax.ClassName)

fun highlightCodeLine(line: String, language: String = ""): AnnotatedString {
    val displayText = line.ifEmpty { " " }
    if (line.isEmpty()) return AnnotatedString(displayText)

    return buildAnnotatedString {
        append(displayText)

        if (language in SECTION_LANGUAGES && line.trim().startsWith("[") && line.trim().endsWith("]")) {
            addStyle(CLASS_NAME_STYLE, 0, line.length)
            return@buildAnnotatedString
        }

        var cursor = 0
        if (language in KEY_LANGUAGES) {
            val keyMatch = KEY_LINE_REGEX.find(line)
            if (keyMatch != null) {
                val keyStart = keyMatch.groupValues[1].length
                val keyLen = keyMatch.groupValues[2].length
                addStyle(BUILTIN_STYLE, keyStart, keyStart + keyLen)
                cursor = keyMatch.range.last + 1
            }
        }

        CODE_TOKEN_REGEX.findAll(line, startIndex = cursor).forEach { m ->
            val token = m.value
            val tokenStart = m.range.first
            val tokenEnd = m.range.last + 1

            val style = when {
                token.startsWith("//") || token.startsWith("#") || token.startsWith("/*") -> COMMENT_STYLE
                token.startsWith("\"") || token.startsWith("'") || token.startsWith("`") -> STRING_STYLE
                token.firstOrNull()?.isDigit() == true -> NUMBER_STYLE
                token in CODE_KEYWORDS -> KEYWORD_STYLE
                token.firstOrNull()?.isUpperCase() == true -> CLASS_NAME_STYLE
                else -> {
                    var j = m.range.last + 1
                    while (j < line.length && line[j].isWhitespace()) j++
                    if (j < line.length && line[j] == '(') BUILTIN_STYLE else null
                }
            }
            if (style != null) {
                addStyle(style, tokenStart, tokenEnd)
            }
        }
    }
}

fun highlightCode(code: String, language: String = ""): AnnotatedString {
    val lines = code.split("\n")
    return buildAnnotatedString {
        lines.forEachIndexed { index, line ->
            if (index > 0) append("\n")
            append(highlightCodeLine(line, language))
        }
    }
}

