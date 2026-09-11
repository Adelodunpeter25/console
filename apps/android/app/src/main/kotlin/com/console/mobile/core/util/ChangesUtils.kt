package com.console.mobile.core.util

import com.console.mobile.data.model.GitFileEntry

sealed interface ChangesRow {
    data class Folder(val key: String, val name: String, val additions: Int, val deletions: Int, val count: Int) : ChangesRow
    data class File(val key: String, val path: String, val name: String, val rel: String, val status: String, val additions: Int, val deletions: Int) : ChangesRow
}

fun statusLetter(s: String?): String {
    val u = (s ?: "").uppercase()
    return when (u) {
        "A" -> "A"
        "D" -> "D"
        "R", "C" -> "R"
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

fun sumTotals(changes: List<GitFileEntry>): ChangeTotals =
    ChangeTotals(changes.size, changes.sumOf { it.additions ?: 0 }, changes.sumOf { it.deletions ?: 0 })

fun buildRows(files: List<GitFileEntry>, collapsed: Set<String>, repoPath: String?): List<ChangesRow> {
    val groups = linkedMapOf<String, MutableList<GitFileEntry>>()
    for (c in files) {
        val d = dirOf(c.path).ifEmpty { "." }
        groups.getOrPut(d) { mutableListOf() }.add(c)
    }
    val out = mutableListOf<ChangesRow>()
    for (dir in groups.keys.sorted()) {
        val fs = groups[dir]!!
        out.add(ChangesRow.Folder("dir:$dir", dir, fs.sumOf { it.additions ?: 0 }, fs.sumOf { it.deletions ?: 0 }, fs.size))
        if (collapsed.contains(dir)) continue
        for (f in fs.sortedBy { it.path }) {
            out.add(ChangesRow.File("file:${f.path}", f.path, baseOf(f.path), stripRepoPrefix(f.path, repoPath), f.status, f.additions ?: 0, f.deletions ?: 0))
        }
    }
    return out
}
