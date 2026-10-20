package com.console.mobile.core.util

sealed class ComposerTrigger {
    abstract val start: Int
    abstract val query: String

    data class Slash(override val start: Int, override val query: String) : ComposerTrigger()
    data class Mention(override val start: Int, override val query: String) : ComposerTrigger()
}

/**
 * Detects an active `/`-command or `@`-mention trigger ending at [cursor].
 * `/` counts at the start of the message or after whitespace, matching the
 * desktop app; `@` counts anywhere as long as there's no whitespace between it
 * and the cursor.
 */
fun detectComposerTrigger(text: String, cursor: Int): ComposerTrigger? {
    if (cursor <= 0 || cursor > text.length) return null

    // The nearest slash that starts a command wins, so a later command in the
    // same message replaces only its own `/query` when accepted.
    for (index in cursor - 1 downTo 0) {
        if (text[index] != '/') continue
        if (index != 0 && !text[index - 1].isWhitespace()) continue
        val query = text.substring(index + 1, cursor)
        if (query.all(::isValidCommandCharacter)) {
            return ComposerTrigger.Slash(start = index, query = query)
        }
        break
    }

    val at = text.lastIndexOf('@', startIndex = cursor - 1)
    if (at >= 0) {
        val span = text.substring(at, cursor)
        if (!span.contains(Regex("\\s"))) {
            return ComposerTrigger.Mention(start = at, query = span.removePrefix("@"))
        }
    }

    return null
}

private fun isValidCommandCharacter(character: Char): Boolean =
    character in 'a'..'z' || character in 'A'..'Z' || character in '0'..'9' ||
        character == '_' || character == '-' || character == ':'
