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

/** Distinct mention paths, for the run request's `contextFiles`. */
fun mentionPaths(text: String): List<String> = parseFileMentions(text).map { it.path }.distinct()
