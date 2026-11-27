package com.console.mobile

import com.console.mobile.core.util.buildRows
import com.console.mobile.core.util.changeSummary
import com.console.mobile.core.util.parentLabel
import console.v1.SessionFileChange
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The changes list is flat: one row per file across the whole session, sorted
 * by path. The server returns per-turn rows newest-first, so a file touched
 * over several turns still shows once, with the latest turn's numbers (the
 * same choice the desktop's All Turns scope makes).
 */
class ChangesRowsTest {
    private fun change(path: String, turn: Int, adds: Int, dels: Int = 0) =
        SessionFileChange(
            path = path, turn_index = turn, status = "modified",
            additions = adds, deletions = dels, updated_at = 0,
        )

    @Test
    fun oneRowPerPathKeepsTheNewestTurn() {
        val rows = buildRows(
            listOf(change("src/a.ts", 3, adds = 10), change("src/a.ts", 1, adds = 4)),
            repoPath = null,
        )
        assertEquals(1, rows.size)
        assertEquals(3, rows[0].turnIndex)
        assertEquals(10, rows[0].additions)
    }

    @Test
    fun rowsAreFlatAndSortedByPath() {
        val rows = buildRows(
            listOf(change("src/b.ts", 1, adds = 1), change("lib/z.ts", 1, adds = 1), change("src/a.ts", 1, adds = 1)),
            repoPath = null,
        )
        assertEquals(listOf("lib/z.ts", "src/a.ts", "src/b.ts"), rows.map { it.path })
    }

    @Test
    fun rowsStripTheRepoPrefix() {
        val rows = buildRows(listOf(change("/repo/src/a.ts", 1, adds = 1)), repoPath = "/repo")
        assertEquals("src/a.ts", rows.single().rel)
        assertEquals("a.ts", rows.single().name)
    }

    @Test
    fun parentLabelKeepsTheLastTwoFolders() {
        assertEquals("editor-ui/src/", parentLabel("crates/editor-ui/src/diff_view.rs"))
        assertEquals("src/", parentLabel("src/a.ts"))
        assertEquals("", parentLabel("README.md"))
    }

    @Test
    fun summaryCountsUniqueFilesButSumsAllTurnChurn() {
        val totals = changeSummary(listOf(change("a.ts", 3, adds = 10), change("a.ts", 1, adds = 4)))
        assertEquals(1, totals.files)
        assertEquals(14, totals.additions)
    }
}
