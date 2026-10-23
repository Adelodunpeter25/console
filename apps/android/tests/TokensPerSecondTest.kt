package com.console.mobile

import com.console.mobile.core.util.formatTokensPerSecond
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Footer throughput label. The measurement and its gating live on the Go
 * server and the number-to-text step lives in `console-shared`
 * (`throughput_test.rs`), shared with the desktop. This covers the Android
 * seam only: unmeasurable turns hide the metric, and a loaded library agrees
 * with the Rust formatter.
 */
class TokensPerSecondTest {
    @Test
    fun hidesUnmeasurableTurns() {
        assertNull(formatTokensPerSecond(null))
        assertNull(formatTokensPerSecond(0.0))
        assertNull(formatTokensPerSecond(-3.0))
        assertNull(formatTokensPerSecond(Double.NaN))
        assertNull(formatTokensPerSecond(Double.POSITIVE_INFINITY))
    }

    @Test
    fun whenLoadedItFormatsOneDecimalWithUnit() {
        if (!SharedNative.available) return
        assertEquals("42.1 tok/s", formatTokensPerSecond(42.06))
        assertEquals("0.4 tok/s", formatTokensPerSecond(0.4))
    }
}
