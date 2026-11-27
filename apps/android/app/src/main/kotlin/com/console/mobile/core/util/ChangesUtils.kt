package com.console.mobile.core.util

import console.v1.SessionFileChange

sealed interface ChangesRow {
    data class Folder(val key: String, val name: String, val additions: Int, val deletions: Int, val count: Int) : ChangesRow
    data class File(val key: String, val path: String, val name: String, val rel: String, val status: String, val additions: Int, val deletions: Int, val turnIndex: Int = 0) : ChangesRow
}

fun dirOf(path: String): String {
    val i = path.lastIndexOf('/')
    return if (i > 0) path.substring(0, i) else ""
}

fun baseOf(path: String): String {
    val i = path.lastIndexOf('/')
    return if (i >= 0) path.substring(i + 1) else path
}

fun stripRepoPrefix(path: String, repoPath: String?): String {
    if (!repoPath.isNullOrEmpty() && path.startsWith("$repoPath/")) return path.substring(repoPath.length + 1)
    return path
}

data class ChangeTotals(val files: Int, val additions: Int, val deletions: Int)

/**
 * Totals for a changes summary: one file per unique path (a file edited over
 * several turns is still one file), with the churn summed across every row.
 */
fun changeSummary(changes: List<SessionFileChange>): ChangeTotals =
    ChangeTotals(
        files = changes.distinctBy { it.path }.size,
        additions = changes.sumOf { it.additions },
        deletions = changes.sumOf { it.deletions },
    )

/** 999 -> "999", 4279 -> "4.3k", 1_250_000 -> "1.3M"; one decimal, half up, no trailing ".0". */
fun formatCompactCount(n: Int): String {
    if (n < 1_000) return n.toString()
    var tenths: Int
    var suffix: String
    if (n < 1_000_000) {
        tenths = (n + 50) / 100
        suffix = "k"
    } else {
        tenths = ((n.toLong() + 50_000L) / 100_000L).toInt()
        suffix = "M"
    }
    // 999_950 rounds up to 1000.0k, which reads better as 1M.
    if (suffix == "k" && tenths >= 10_000) {
        tenths = 10
        suffix = "M"
    }
    val whole = tenths / 10
    val frac = tenths % 10
    return if (frac == 0) "$whole$suffix" else "$whole.$frac$suffix"
}

/** One arc of a ring: start and length as fractions of the circle, 0.0 at 12 o'clock. */
data class RingSpan(val start: Float, val len: Float)

/**
 * Splits a circle between parts sized by weights, one span per weight in order.
 * A zero weight gets an empty span. With two or more non-zero parts a gap
 * separates them and each keeps at least [min], so a tiny share stays visible;
 * a single part fills the whole circle.
 */
fun splitRing(weights: List<Float>, gap: Float, min: Float): List<RingSpan> {
    val live = weights.count { it > 0f }
    val spans = MutableList(weights.size) { RingSpan(0f, 0f) }
    if (live == 0) return spans
    if (live == 1) {
        spans[weights.indexOfFirst { it > 0f }] = RingSpan(0f, 1f)
        return spans
    }
    val total = weights.filter { it > 0f }.sum()
    val usable = 1f - gap * live
    val lens = weights.map { if (it > 0f) maxOf(it / total * usable, min) else 0f }
    val scale = usable / lens.sum()
    var cursor = gap / 2f
    lens.forEachIndexed { i, len ->
        if (len > 0f) {
            val scaled = len * scale
            spans[i] = RingSpan(cursor, scaled)
            cursor += scaled + gap
        }
    }
    return spans
}

fun buildRows(files: List<SessionFileChange>, collapsed: Set<String>, repoPath: String?): List<ChangesRow> {
    // One row per file across the whole session: the server returns rows
    // newest-first, so keeping the first for a path shows the latest turn's
    // diff — the same choice the desktop's All Turns scope makes.
    val unique = files.distinctBy { it.path }
    val groups = linkedMapOf<String, MutableList<SessionFileChange>>()
    for (c in unique) {
        val d = dirOf(c.path).ifEmpty { "." }
        groups.getOrPut(d) { mutableListOf() }.add(c)
    }
    val out = mutableListOf<ChangesRow>()
    for (dir in groups.keys.sorted()) {
        val fs = groups[dir]!!
        out.add(ChangesRow.Folder("dir:$dir", dir, fs.sumOf { it.additions }, fs.sumOf { it.deletions }, fs.size))
        if (collapsed.contains(dir)) continue
        for (f in fs.sortedBy { it.path }) {
            out.add(ChangesRow.File("file:${f.path}", f.path, baseOf(f.path), stripRepoPrefix(f.path, repoPath), f.status, f.additions, f.deletions, f.turn_index))
        }
    }
    return out
}
