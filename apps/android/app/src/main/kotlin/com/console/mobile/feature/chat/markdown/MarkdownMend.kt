package com.console.mobile.feature.chat.markdown

/**
 * Closes hanging inline markers in a *streaming* markdown tail.
 * Port of apps/desktop/crates/console-ui/src/markdown/mend.rs.
 *
 * While a paragraph streams, an unclosed `**bold`, `*em`, `` `code ``,
 * `~~strike` or `[text](half-url` parses as literal text. When the closer
 * finally arrives the marker characters vanish and the run restyles, so wrap
 * points shift and the paragraph's tail visibly reflows — a jitter that lands
 * on every emphasis in a streamed response.
 *
 * Appending synthetic closers to the *display* parse keeps styling stable from
 * the moment content follows an opener. Only the display parse sees mended text
 * ([MarkdownDocument]); the canonical blocks are untouched, so a marker that
 * genuinely never closes settles honestly — one flip when the response
 * completes — instead of jittering throughout.
 *
 * The scanner is deliberately approximate: it prefers "stable and close to the
 * final parse" over exact CommonMark delimiter resolution, because any
 * mid-stream misjudgement is repaired by the very next append or by the
 * settle. Cost is one pass over the tail, and `null` — zero further work —
 * whenever nothing hangs, which is the overwhelmingly common case.
 */

/** Destination for a link whose URL is still streaming. Styled like a link, not clickable. */
internal const val PENDING_LINK_URL = "console:pending-link"

/** Zero-width space appended to defuse a would-be setext underline. */
private const val ZERO_WIDTH_SPACE = '\u200B'

/** One unclosed emphasis-family delimiter run. */
private class OpenDelimiter(val marker: Char, var owed: Int, val openedAt: Int)

/**
 * Repair hanging inline markers. Returns `null` when nothing needs repair.
 */
internal fun closeHanging(text: String): String? {
    val length = text.length
    fun at(index: Int): Char? = if (index in 0 until length) text[index] else null
    fun runLength(start: Int): Int {
        val marker = text[start]
        var end = start
        while (end < length && text[end] == marker) end++
        return end - start
    }

    val delimiters = ArrayList<OpenDelimiter>()
    val brackets = ArrayList<Int>()
    // Open inline code span: (backtick run length, index of its first content char).
    var code: Pair<Int, Int>? = null
    var lastContent: Int? = null
    // Index of the `]` whose `](…` URL runs off the end of the tail.
    var pendingUrl: Int? = null

    var index = 0
    while (index < length) {
        val ch = text[index]

        if (code == null && ch == '\\') {
            // An escape and its escapee are both literal, but the escapee is
            // still content that can justify closing an opener.
            if (index + 1 < length) lastContent = index + 1
            index += 2
            continue
        }

        if (ch == '`') {
            val run = runLength(index)
            val open = code
            when {
                // A span closes only on a run of exactly the opening length.
                // Its closing backticks are content: an emphasis closer owed
                // from before the span must land after them, not inside it.
                open != null && run == open.first -> {
                    code = null
                    lastContent = index + run - 1
                }
                open != null -> lastContent = index + run - 1
                else -> code = run to (index + run)
            }
            index += run
            continue
        }

        if (code != null) {
            lastContent = index
            index++
            continue
        }

        when (ch) {
            '*', '_', '~' -> {
                val run = runLength(index)
                scanDelimiter(delimiters, ch, run, index, { lastContent }, { lastContent = it }, ::at)
                index += run
            }
            '[' -> {
                brackets.add(index)
                index++
            }
            ']' -> {
                val open = brackets.removeLastOrNull()
                if (open != null) {
                    // Emphasis opened inside a completed `[…]` and never closed
                    // there stays literal, as the final parse will also decide.
                    delimiters.removeAll { it.openedAt >= open }
                    if (at(index + 1) == '(') {
                        // Consume the URL through its balanced `)`.
                        var scan = index + 2
                        var depth = 0
                        while (true) {
                            when (at(scan)) {
                                '(' -> depth++
                                ')' -> {
                                    if (depth == 0) break
                                    depth--
                                }
                                null -> {
                                    pendingUrl = index
                                    break
                                }
                                else -> Unit
                            }
                            scan++
                        }
                        if (pendingUrl != null) break
                        lastContent = scan
                        index = scan + 1
                        continue
                    }
                }
                lastContent = index
                index++
            }
            else -> {
                if (ch.isWhitespace()) {
                    index++
                } else {
                    lastContent = index
                    index++
                }
            }
        }
    }

    // Closers must sit immediately after the last content character: a `**`
    // preceded by whitespace is left-flanking, so it would open a *new* run
    // rather than close the hanging one, and the repair would never converge.
    val contentEnd = lastContent?.plus(1) ?: text.length

    val openUrl = pendingUrl
    if (openUrl != null) {
        // The URL never renders, so a bare `[text` and a half-typed
        // `[text](http` mend identically: the link text shows styled at once
        // and the settling URL cannot collapse the line.
        val cut = openUrl
        val closers = StringBuilder()
        closeDelimiters(closers, delimiters, lastContent, cut)
        val mended = StringBuilder(cut + closers.length + PENDING_LINK_URL.length + 4)
        val end = minOf(contentEnd, cut)
        mended.append(text, 0, end)
        mended.append(closers)
        mended.append(text, end, cut)
        mended.append("](")
        mended.append(PENDING_LINK_URL)
        mended.append(')')
        return mended.toString()
    }

    val closers = StringBuilder()
    val open = code
    val last = lastContent
    if (open != null && last != null && last >= open.second) {
        repeat(open.first) { closers.append('`') }
    }
    closeDelimiters(closers, delimiters, lastContent, length)
    val bracket = brackets.lastOrNull()
    if (bracket != null && last != null && last > bracket) {
        closers.append("](")
        closers.append(PENDING_LINK_URL)
        closers.append(')')
    }

    val setextGuard = needsSetextGuard(text)
    if (closers.isEmpty() && !setextGuard) return null

    val mended = StringBuilder(text.length + closers.length + 3)
    mended.append(text, 0, contentEnd)
    mended.append(closers)
    mended.append(text, contentEnd, text.length)
    if (setextGuard) mended.append(ZERO_WIDTH_SPACE)
    return mended.toString()
}

/**
 * Append closers for every still-open delimiter, innermost first, skipping any
 * opener that has no content after it yet (`**` alone must stay literal, not
 * become an empty bold).
 */
private fun closeDelimiters(
    out: StringBuilder,
    delimiters: List<OpenDelimiter>,
    lastContent: Int?,
    limit: Int,
) {
    for (delimiter in delimiters.asReversed()) {
        if (delimiter.openedAt >= limit) continue
        val hasContent = lastContent != null && lastContent >= delimiter.openedAt
        if (!hasContent) continue
        repeat(delimiter.owed) { out.append(delimiter.marker) }
    }
}

/**
 * Classify one emphasis-family run as an opener or a closer and update the
 * open-delimiter stack.
 */
private fun scanDelimiter(
    delimiters: MutableList<OpenDelimiter>,
    marker: Char,
    run: Int,
    index: Int,
    lastContent: () -> Int?,
    setLast: (Int) -> Unit,
    at: (Int) -> Char?,
) {
    // A lone `~` is literal in GFM: strikethrough needs `~~`.
    if (marker == '~' && run < 2) {
        setLast(index + run - 1)
        return
    }

    val before = if (index > 0) at(index - 1) else null
    val after = at(index + run)
    val canClose = before != null && !before.isWhitespace()
    val canOpen = after != null && !after.isWhitespace()

    val position = if (canClose) delimiters.indexOfLast { it.marker == marker } else -1
    val content = lastContent()
    if (position >= 0 && content != null && content >= delimiters[position].openedAt) {
        val owed = delimiters[position].owed
        if (run >= owed) {
            // Fully closed; any surplus markers are rare enough to ignore.
            delimiters.subList(position, delimiters.size).clear()
        } else {
            // A half-streamed closer (`**a*`): we still owe the remainder.
            delimiters[position].owed = owed - run
            delimiters.subList(position + 1, delimiters.size).clear()
        }
        setLast(index + run - 1)
        return
    }

    // `_` does not open inside a word, matching CommonMark's intraword rule.
    if (marker == '_' && before != null && before.isLetterOrDigit()) {
        setLast(index + run - 1)
        return
    }

    if (canOpen) {
        delimiters.add(OpenDelimiter(marker, run, index + run))
    } else {
        setLast(index + run - 1)
    }
}

/**
 * A final line of only `-` or `=` directly under a text line is a setext
 * heading underline, so a streaming list item flashes its whole paragraph as a
 * heading for one chunk. True when that flash is imminent.
 */
private fun needsSetextGuard(text: String): Boolean {
    if (text.endsWith("\n")) return false
    val lines = text.lines()
    val last = lines.lastOrNull() ?: return false
    val trimmed = last.trimEnd()
    if (trimmed.isEmpty()) return false
    if (!(trimmed.all { it == '-' } || trimmed.all { it == '=' })) return false
    val previous = lines.getOrNull(lines.size - 2)?.trim() ?: return false
    if (previous.isEmpty()) return false
    return previous.first() !in charArrayOf('-', '=', '#', '>', '`')
}

