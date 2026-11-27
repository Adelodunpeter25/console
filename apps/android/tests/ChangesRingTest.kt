package com.console.mobile

import com.console.mobile.core.util.RingSpan
import com.console.mobile.core.util.formatCompactCount
import com.console.mobile.core.util.splitRing
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The changed-files pill shows compact totals ("+4.3k") and a ring split by
 * lines added and removed. Both helpers mirror the desktop's, so the two
 * clients read the same numbers and draw the same shape.
 */
class ChangesRingTest {
    private val eps = 1e-4f

    private fun close(expected: Float, actual: Float) = assertTrue("$expected vs $actual", kotlin.math.abs(expected - actual) < eps)

    @Test
    fun smallNumbersAreUnchanged() {
        assertEquals("0", formatCompactCount(0))
        assertEquals("129", formatCompactCount(129))
        assertEquals("999", formatCompactCount(999))
    }

    @Test
    fun thousandsRoundToOneDecimal() {
        assertEquals("1k", formatCompactCount(1_000))
        assertEquals("1.2k", formatCompactCount(1_150))
        assertEquals("1.6k", formatCompactCount(1_639))
        assertEquals("4.3k", formatCompactCount(4_279))
        assertEquals("27k", formatCompactCount(27_000))
    }

    @Test
    fun roundingUpNeverReadsAsAThousandK() {
        assertEquals("999.9k", formatCompactCount(999_949))
        assertEquals("1M", formatCompactCount(999_950))
    }

    @Test
    fun millionsUseAnMSuffix() {
        assertEquals("1M", formatCompactCount(1_000_000))
        assertEquals("1.3M", formatCompactCount(1_250_000))
    }

    @Test
    fun noWeightDrawsNothing() {
        assertEquals(listOf(RingSpan(0f, 0f), RingSpan(0f, 0f)), splitRing(listOf(0f, 0f), 0.06f, 0.06f))
    }

    @Test
    fun singleSideFillsTheWholeRing() {
        assertEquals(listOf(RingSpan(0f, 1f), RingSpan(0f, 0f)), splitRing(listOf(10f, 0f), 0.06f, 0.06f))
        assertEquals(listOf(RingSpan(0f, 0f), RingSpan(0f, 1f)), splitRing(listOf(0f, 7f), 0.06f, 0.06f))
    }

    @Test
    fun twoSidesShareTheRingWithAGapBetweenThem() {
        val (a, b) = splitRing(listOf(50f, 50f), 0.06f, 0.06f)
        close(a.len, b.len)
        close(1f, a.len + b.len + 0.12f)
        close(0.03f, a.start)
        close(a.start + a.len + 0.06f, b.start)
    }

    @Test
    fun aTinyShareKeepsAVisibleArc() {
        val (big, tiny) = splitRing(listOf(4279f, 1f), 0.06f, 0.06f)
        assertTrue(tiny.len >= 0.05f)
        close(1f, big.len + tiny.len + 0.12f)
    }

    @Test
    fun arcsFollowTheWeightRatio() {
        val (a, b) = splitRing(listOf(3f, 1f), 0f, 0f)
        close(0.75f, a.len)
        close(0.25f, b.len)
        close(0.75f, b.start)
    }
}
