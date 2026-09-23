package com.console.mobile.core.util

sealed class ComposerTrigger {
    abstract val start: Int
    abstract val query: String

    data class Slash(override val start: Int, override val query: String) : ComposerTrigger()
    data class Mention(override val start: Int, override val query: String) : ComposerTrigger()
}

/**
 * Detects an active `/`-command or `@`-mention trigger ending at [cursor].
 * `/` only counts at the very start of the message (mirrors the desktop app);
 * `@` counts anywhere as long as there's no whitespace between it and the cursor.
 */
fun detectComposerTrigger(text: String, cursor: Int): ComposerTrigger? {
    if (cursor <= 0 || cursor > text.length) return null

    if (text.startsWith("/")) {
        val span = text.substring(0, cursor)
        if (!span.contains(Regex("\\s"))) {
            return ComposerTrigger.Slash(start = 0, query = span.removePrefix("/"))
        }
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
