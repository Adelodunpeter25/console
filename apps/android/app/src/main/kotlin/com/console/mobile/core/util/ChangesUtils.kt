package com.console.mobile.core.util

import console.v1.SessionFileChange

/** One changed file in the flat changes list. */
data class ChangesRow(
    val key: String,
    val path: String,
    val name: String,
    val rel: String,
    val status: String,
    val additions: Int,
    val deletions: Int,
    val turnIndex: Int = 0,
)

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

/**
 * The changes list: one row per file across the whole session, flat and
 * sorted by path so files from one folder still sit together. The server
 * returns rows newest-first, so keeping the first for a path shows the
 * latest turn's diff, the same choice the desktop's All Turns scope makes.
 */
fun buildRows(files: List<SessionFileChange>, repoPath: String?): List<ChangesRow> =
    files.distinctBy { it.path }
        .sortedBy { it.path }
        .map { f ->
            ChangesRow("file:${f.path}", f.path, baseOf(f.path), stripRepoPrefix(f.path, repoPath), f.status, f.additions, f.deletions, f.turn_index)
        }

/** The last two folders of a repo-relative path, for the dim hint beside a file name; "" at the repo root. */
fun parentLabel(rel: String): String {
    val dir = dirOf(rel)
    if (dir.isEmpty()) return ""
    return dir.split('/').takeLast(2).joinToString("/") + "/"
}
