package com.console.mobile.core.util

/**
 * `@`-mention ranges in composer/bubble text. Desktop parity
 * (file_mention_chip.rs): a mention starts at `@` on a whitespace (or
 * text-start) boundary and runs to the next whitespace, so emails like
 * `a@b.com` never match.
 */
data class FileMention(val range: IntRange, val path: String) {
    val label: String get() = path.substringAfterLast('/')
}

private val trailingPunctuation = setOf(',', '.', ';', ':', '!', '?')

fun parseFileMentions(text: String): List<FileMention> {
    val out = mutableListOf<FileMention>()
    var i = 0
    while (i < text.length) {
        if (text[i] == '@' && (i == 0 || text[i - 1].isWhitespace())) {
            var j = i + 1
            while (j < text.length && !text[j].isWhitespace()) j++
            var end = j
            while (end > i + 1 && text[end - 1] in trailingPunctuation) end--
            if (end > i + 1) out += FileMention(i until end, text.substring(i + 1, end))
            i = j
        } else {
            i++
        }
    }
    return out
}


/**
 * `$` chat-ref ranges in composer text, matching the server's
 * `ParseDollarRefs` (shared cases: proto/testdata/session/chat_refs.json):
 * `$` at start of text or after whitespace/opening punctuation, then ASCII
 * letters, digits, `-` or `_`. Edge dashes/underscores are trimmed from the
 * slug, and a digit-led token only counts when it looks like an id (contains
 * `-`, at least 8 long), so prices stay plain text.
 */
data class ChatMention(val range: IntRange, val slug: String)

private fun isRefChar(c: Char) = c in 'a'..'z' || c in 'A'..'Z' || c in '0'..'9' || c == '_' || c == '-'

fun parseChatMentions(text: String): List<ChatMention> {
    val out = mutableListOf<ChatMention>()
    var i = 0
    while (i < text.length) {
        val boundary = i == 0 || text[i - 1].isWhitespace() || text[i - 1] in "([{<\"'"
        if (text[i] == '$' && boundary) {
            var j = i + 1
            while (j < text.length && isRefChar(text[j])) j++
            val slug = text.substring(i + 1, j).trim('-', '_')
            val digitLed = slug.firstOrNull()?.isDigit() == true
            if (slug.isNotEmpty() && !(digitLed && !(slug.contains('-') && slug.length >= 8))) {
                out += ChatMention(i until j, slug)
            }
            i = j
        } else {
            i++
        }
    }
    return out
}


/**
 * Slash skill mentions `/skill-name` in composer text. Starts with `/` at
 * start of text or after whitespace, and extends across valid command chars.
 */
data class SkillMention(val range: IntRange, val name: String)

fun parseSkillMentions(text: String): List<SkillMention> {
    val out = mutableListOf<SkillMention>()
    var i = 0
    while (i < text.length) {
        if (text[i] == '/' && (i == 0 || text[i - 1].isWhitespace())) {
            var j = i + 1
            while (j < text.length && !text[j].isWhitespace() && text[j] !in trailingPunctuation) j++
            if (j > i + 1) {
                out += SkillMention(i until j, text.substring(i + 1, j))
            }
            i = j
        } else {
            i++
        }
    }
    return out
}

/**
 * Distinct mention paths, for the run request's `contextFiles`. Skills and
 * chat refs ride along as `skill:<name>` / `chat:<slug>` (desktop parity) so
 * sent messages can redraw them as chips; the server keeps them out of the
 * agent text. Slash words containing `/` are paths, not skills.
 */
fun mentionPaths(text: String): List<String> =
    (parseFileMentions(text).map { it.path } +
        parseSkillMentions(text).filter { '/' !in it.name }.map { "skill:${it.name}" } +
        parseChatMentions(text).map { "chat:${it.slug}" }).distinct()
