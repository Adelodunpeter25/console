package com.console.mobile.core.util

import console.v1.SessionFileChange

sealed interface ChangesRow {
    data class Folder(val key: String, val name: String, val additions: Int, val deletions: Int, val count: Int) : ChangesRow
    data class File(val key: String, val path: String, val name: String, val rel: String, val status: String, val additions: Int, val deletions: Int, val reviewed: Boolean = false, val turnIndex: Int = 0) : ChangesRow
}

fun statusLetter(s: String?): String {
    val u = (s ?: "").uppercase()
    return when (u) {
        "A", "ADDED" -> "A"
        "D", "DELETED" -> "D"
        "R", "C", "RENAMED" -> "R"
        "U" -> "U"
        "?" -> "?"
        else -> "M"
    }
}

fun statusColorHex(s: String?): String = when (statusLetter(s)) {
    "A" -> "#34d399"
    "D" -> "#f87171"
    "R" -> "#38bdf8"
    "U" -> "#fb7185"
    "?" -> "#a1a1aa"
    else -> "#facc15"
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

fun sumTotals(changes: List<SessionFileChange>): ChangeTotals =
    ChangeTotals(changes.size, changes.sumOf { it.additions }, changes.sumOf { it.deletions })

fun buildRows(files: List<SessionFileChange>, collapsed: Set<String>, repoPath: String?): List<ChangesRow> {
    val groups = linkedMapOf<String, MutableList<SessionFileChange>>()
    for (c in files) {
        val d = dirOf(c.path).ifEmpty { "." }
        groups.getOrPut(d) { mutableListOf() }.add(c)
    }
    val out = mutableListOf<ChangesRow>()
    for (dir in groups.keys.sorted()) {
        val fs = groups[dir]!!
        out.add(ChangesRow.Folder("dir:$dir", dir, fs.sumOf { it.additions }, fs.sumOf { it.deletions }, fs.size))
        if (collapsed.contains(dir)) continue
        for (f in fs.sortedBy { it.path }) {
            // Same path can repeat across turns: key on both.
            out.add(ChangesRow.File("file:${f.path}#${f.turn_index}", f.path, baseOf(f.path), stripRepoPrefix(f.path, repoPath), f.status, f.additions, f.deletions, f.reviewed, f.turn_index))
        }
    }
    return out
}
