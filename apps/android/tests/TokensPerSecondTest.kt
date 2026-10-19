package com.console.mobile

import com.console.mobile.core.util.formatTokensPerSecond
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Footer throughput label. The measurement and its gating live on the Go
 * server; this covers only the number-to-text step the bubble renders.
 */
class TokensPerSecondTest {
    @Test
    fun formatsOneDecimalWithUnit() {
        assertEquals("42.1 tok/s", formatTokensPerSecond(42.06))
        assertEquals("120.0 tok/s", formatTokensPerSecond(120.0))
    }

    @Test
    fun keepsSubOneRatesVisible() {
        assertEquals("0.4 tok/s", formatTokensPerSecond(0.4))
    }

    @Test
    fun hidesUnmeasurableTurns() {
        assertNull(formatTokensPerSecond(null))
        assertNull(formatTokensPerSecond(0.0))
        assertNull(formatTokensPerSecond(-3.0))
        assertNull(formatTokensPerSecond(Double.NaN))
        assertNull(formatTokensPerSecond(Double.POSITIVE_INFINITY))
    }
}
