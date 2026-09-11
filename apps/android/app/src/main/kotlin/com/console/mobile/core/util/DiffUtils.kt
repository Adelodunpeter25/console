package com.console.mobile.core.util

enum class DiffLineType { Added, Removed, Context }

data class DiffLine(val type: DiffLineType, val text: String, val oldLineNo: Int? = null, val newLineNo: Int? = null)
data class DiffResult(val lines: List<DiffLine>, val addedCount: Int, val removedCount: Int)

/** Port of parseUnifiedDiff in utils/changes.ts */
fun parseUnifiedDiff(diff: String): DiffResult {
    val out = mutableListOf<DiffLine>()
    var added = 0
    var removed = 0
    for (l in diff.split("\n")) {
        if (l.startsWith("+++") || l.startsWith("---") || l.startsWith("@@") || l.startsWith("diff ") || l.startsWith("index ")) continue
        when {
            l.startsWith("+") -> { out.add(DiffLine(DiffLineType.Added, l.substring(1))); added++ }
            l.startsWith("-") -> { out.add(DiffLine(DiffLineType.Removed, l.substring(1))); removed++ }
            else -> out.add(DiffLine(DiffLineType.Context, if (l.startsWith(" ")) l.substring(1) else l))
        }
    }
    return DiffResult(out, added, removed)
}

private fun splitLines(text: String): List<String> {
    if (text.isEmpty()) return emptyList()
    val lines = text.split("\n").toMutableList()
    if (lines.isNotEmpty() && lines.last() == "") lines.removeAt(lines.lastIndex)
    return lines
}

/** Line diff via LCS on lines (small-file friendly; replaces `diff` npm dep). */
fun computeLineDiff(oldContent: String = "", newContent: String = ""): DiffResult {
    val a = splitLines(oldContent)
    val b = splitLines(newContent)
    val n = a.size
    val m = b.size
    // LCS DP (files here are preview-capped at ~512KB but typically small; guard huge inputs)
    if (n * m > 4_000_000) {
        // fallback: treat as full replace
        val lines = mutableListOf<DiffLine>()
        var oldNo = 1
        var newNo = 1
        for (t in a) lines.add(DiffLine(DiffLineType.Removed, t, oldLineNo = oldNo++))
        for (t in b) lines.add(DiffLine(DiffLineType.Added, t, newLineNo = newNo++))
        return DiffResult(lines, b.size, a.size)
    }
    val dp = Array(n + 1) { IntArray(m + 1) }
    for (i in n - 1 downTo 0) for (j in m - 1 downTo 0) {
        dp[i][j] = if (a[i] == b[j]) dp[i + 1][j + 1] + 1 else maxOf(dp[i + 1][j], dp[i][j + 1])
    }
    val lines = mutableListOf<DiffLine>()
    var i = 0
    var j = 0
    var oldNo = 1
    var newNo = 1
    var added = 0
    var removed = 0
    while (i < n && j < m) {
        if (a[i] == b[j]) {
            lines.add(DiffLine(DiffLineType.Context, a[i], oldLineNo = oldNo++, newLineNo = newNo++))
            i++; j++
        } else if (dp[i + 1][j] >= dp[i][j + 1]) {
            lines.add(DiffLine(DiffLineType.Removed, a[i], oldLineNo = oldNo++))
            removed++; i++
        } else {
            lines.add(DiffLine(DiffLineType.Added, b[j], newLineNo = newNo++))
            added++; j++
        }
    }
    while (i < n) { lines.add(DiffLine(DiffLineType.Removed, a[i], oldLineNo = oldNo++)); removed++; i++ }
    while (j < m) { lines.add(DiffLine(DiffLineType.Added, b[j], newLineNo = newNo++)); added++; j++ }
    return DiffResult(lines, added, removed)
}

fun computeNewFileDiff(content: String = ""): DiffResult {
    val raw = splitLines(content)
    val lines = raw.mapIndexed { idx, t -> DiffLine(DiffLineType.Added, t, newLineNo = idx + 1) }
    return DiffResult(lines, lines.size, 0)
}
