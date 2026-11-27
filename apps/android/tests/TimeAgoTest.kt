package com.console.mobile

import com.console.mobile.core.util.formatTimeAgo
import java.text.SimpleDateFormat
import java.util.Locale
import org.junit.Assert.assertEquals
import org.junit.Test

/** The prompt list's age labels follow the desktop's `format_time_ago` boundaries. */
class TimeAgoTest {
    private val now = 1_700_000_000_000L
    private fun ago(seconds: Long) = formatTimeAgo(now - seconds * 1000, now)

    @Test
    fun usesOneCoarseUnitUpToAWeek() {
        assertEquals("just now", ago(59))
        assertEquals("1m ago", ago(60))
        assertEquals("59m ago", ago(3_599))
        assertEquals("1h ago", ago(3_600))
        assertEquals("23h ago", ago(86_399))
        assertEquals("1d ago", ago(86_400))
        assertEquals("6d ago", ago(604_799))
    }

    @Test
    fun olderThanAWeekIsABareDateWithoutAgo() {
        val ts = now - 604_800 * 1000
        val expected = SimpleDateFormat("MMM d", Locale.US).format(java.util.Date(ts))
        assertEquals(expected, formatTimeAgo(ts, now))
    }

    @Test
    fun aFutureTimestampReadsAsJustNow() {
        assertEquals("just now", formatTimeAgo(now + 5_000, now))
    }
}
