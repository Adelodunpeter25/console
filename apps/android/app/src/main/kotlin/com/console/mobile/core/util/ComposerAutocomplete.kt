package com.console.mobile.core.util

sealed class ComposerTrigger {
    abstract val start: Int
    abstract val query: String

    data class Slash(override val start: Int, override val query: String) : ComposerTrigger()
    data class Mention(override val start: Int, override val query: String) : ComposerTrigger()
    data class Chat(override val start: Int, override val query: String) : ComposerTrigger()
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

    val dollar = text.lastIndexOf('$', startIndex = cursor - 1)
    if (dollar >= 0 && (dollar == 0 || text[dollar - 1].isWhitespace() || text[dollar - 1] in "([{<\"'")) {
        val query = text.substring(dollar + 1, cursor)
        // Digit-only queries are prices/amounts, not chat refs (the server
        // only treats a digit-led token as a ref when it looks like an id).
        if (query.all(::isRefCharacter) && !(query.isNotEmpty() && query.all(Char::isDigit))) {
            return ComposerTrigger.Chat(start = dollar, query = query)
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

private fun isRefCharacter(character: Char): Boolean =
    character in 'a'..'z' || character in 'A'..'Z' || character in '0'..'9' ||
        character == '_' || character == '-'

/**
 * Chat-title slug, mirroring the server's SlugifyTitle: lowercase, every
 * run of non-letters/digits collapsed to one '-', trimmed. A `$` ref typed
 * from this slug resolves server-side against the chat's title.
 */
fun slugifyChatTitle(title: String): String {
    val out = StringBuilder()
    var lastDash = true
    for (r in title.lowercase()) {
        if (r.isLetterOrDigit()) {
            out.append(r)
            lastDash = false
        } else if (!lastDash) {
            out.append('-')
            lastDash = true
        }
    }
    return out.toString().trim('-')
}

/**
 * `$` chat-ref suggestions from the same-project session list: the current
 * chat is excluded (the server refuses self-references too), and the query
 * matches the title slug, the title, or the session id prefix.
 */
fun filterChatRefs(
    sessions: List<console.v1.SessionHeader>,
    currentSessionId: String,
    query: String,
): List<console.v1.SessionHeader> {
    val q = query.lowercase()
    return sessions
        .filter { it.id != currentSessionId }
        .filter { s ->
            q.isEmpty() ||
                slugifyChatTitle(s.title ?: "").contains(q) ||
                (s.title ?: "").lowercase().contains(q) ||
                s.id.lowercase().startsWith(q)
        }
        .sortedWith(
            compareByDescending<console.v1.SessionHeader> { s ->
                val slug = slugifyChatTitle(s.title ?: "")
                when {
                    q.isEmpty() -> 0
                    slug.startsWith(q) -> 3
                    s.id.lowercase().startsWith(q) -> 2
                    (s.title ?: "").lowercase().startsWith(q) -> 1
                    else -> 0
                }
            }.thenBy { it.title ?: "" },
        )
}

private fun isValidCommandCharacter(character: Char): Boolean =
    character in 'a'..'z' || character in 'A'..'Z' || character in '0'..'9' ||
        character == '_' || character == '-' || character == ':'
