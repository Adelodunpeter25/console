package com.console.mobile

import com.console.mobile.core.util.ChangesRow
import com.console.mobile.core.util.buildRows
import com.console.mobile.core.util.changeSummary
import console.v1.SessionFileChange
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * The changes list is one row per file across the whole session: the server
 * returns per-turn rows newest-first, so a file touched over several turns
 * still shows once, with the latest turn's numbers (the same choice the
 * desktop's All Turns scope makes).
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
            collapsed = emptySet(), repoPath = null,
        )
        val fileRows = rows.filterIsInstance<ChangesRow.File>()
        assertEquals(1, fileRows.size)
        assertEquals(3, fileRows[0].turnIndex)
        assertEquals(10, fileRows[0].additions)
    }

    @Test
    fun folderTotalsCountUniqueFiles() {
        val rows = buildRows(
            listOf(
                change("src/a.ts", 3, adds = 10),
                change("src/a.ts", 1, adds = 4),
                change("src/b.ts", 3, adds = 2),
            ),
            collapsed = emptySet(), repoPath = null,
        )
        val folder = rows.filterIsInstance<ChangesRow.Folder>().single()
        assertEquals("src", folder.name)
        assertEquals(2, folder.count)
        assertEquals(12, folder.additions)
    }

    @Test
    fun summaryCountsUniqueFilesButSumsAllTurnChurn() {
        val totals = changeSummary(listOf(change("a.ts", 3, adds = 10), change("a.ts", 1, adds = 4)))
        assertEquals(1, totals.files)
        assertEquals(14, totals.additions)
    }
}
