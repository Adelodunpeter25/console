package com.console.mobile.core.util

import com.console.mobile.data.model.GitFileEntry
import com.console.mobile.data.model.UsageAmount
import com.console.mobile.data.model.UsageLimit
import com.console.mobile.data.model.UsageScope
import com.console.mobile.data.model.UsageWindow
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class UtilsTest {
    @Test fun normalizeBackendUrl() {
        assertEquals("http://a:3000", normalizeBackendUrl("a:3000"))
        assertEquals("https://a", normalizeBackendUrl("https://a/"))
        assertNull(normalizeBackendUrl("   "))
    }

    @Test fun folderAndModelNames() {
        assertEquals("console", folderName("/a/b/console/"))
        assertEquals("Claude Opus 4 6", formatModelName("claude-opus-4-6"))
    }

    @Test fun relativeTime() {
        val now = 1_000_000L
        assertEquals("just now", formatRelativeTime(now - 10_000, now))
        assertEquals("5m ago", formatRelativeTime(now - 5 * 60_000, now))
        assertEquals("2h ago", formatRelativeTime(now - 2 * 3600_000, now))
        assertEquals("3d ago", formatRelativeTime(now - 3 * 86400_000, now))
    }

    @Test fun diffUtils() {
        val d = computeLineDiff("a\nb\n", "a\nc\n")
        assertEquals(1, d.addedCount)
        assertEquals(1, d.removedCount)
        assertTrue(d.lines.any { it.type == DiffLineType.Context && it.text == "a" })
        val nd = computeNewFileDiff("x\ny")
        assertEquals(2, nd.addedCount)
        val u = parseUnifiedDiff("+++ b\n+hi\n- bye\n ctx")
        assertEquals(1, u.addedCount)
        assertEquals(1, u.removedCount)
    }

    @Test fun changesRows() {
        val files = listOf(
            GitFileEntry("src/a.ts", "M", false, 3, 1),
            GitFileEntry("src/b.ts", "A", false, 10, 0),
        )
        assertEquals("A", statusLetter("a"))
        assertEquals("M", statusLetter("m"))
        val totals = sumTotals(files)
        assertEquals(2, totals.files)
        assertEquals(13, totals.additions)
        val rows = buildRows(files, emptySet(), null)
        assertEquals(3, rows.size) // 1 folder + 2 files
        val collapsed = buildRows(files, setOf("src"), null)
        assertEquals(1, collapsed.size)
    }

    @Test fun usageHelpers() {
        val limit = UsageLimit("id", "label", UsageScope("antigravity"), UsageWindow("w", "Quota", resetsAt = 3_600_000L + 1_000_000L), UsageAmount(usedFraction = 0.6, unit = "percent"))
        assertEquals("warning", statusForLimit(limit))
        assertEquals("#fbbf24", colorForLimit(limit))
        assertEquals(60, getUsedPercent(limit))
        assertEquals("Quota · resets in 1h 0m", formatWindowLabel(limit, 1_000_000L))
        assertNull(formatResetsAt(null))
    }

    @Test fun toolHelpers() {
        assertEquals("typescript", langFromPath("a/b.ts"))
        assertEquals("dockerfile", langFromPath("Dockerfile"))
        assertEquals("Read File", getToolLabel("readFile"))
        assertEquals("custom", getToolLabel("custom"))
        assertEquals("b.ts", getFileName("/a/b.ts"))
        assertEquals("b.ts", toRelativePath("/root/b.ts", "/root"))
    }

    @Test fun fsGating() {
        assertTrue(com.console.mobile.data.model.isLockFileName("bun.lockb"))
        assertTrue(com.console.mobile.data.model.isBinaryFileName("a.apk"))
        assertTrue(com.console.mobile.data.model.isPreviewableImageName("a.png"))
        assertTrue(com.console.mobile.data.model.isSvgFileName("a.svg"))
        assertTrue(com.console.mobile.data.model.getFilePreviewBlock("bun.lock", 10) != null)
        assertTrue(com.console.mobile.data.model.getFilePreviewBlock("big.txt", 600 * 1024) != null)
        assertTrue(com.console.mobile.data.model.getFilePreviewBlock("ok.txt", 10) == null)
    }
}
