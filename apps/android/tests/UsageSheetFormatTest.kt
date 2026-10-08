package com.console.mobile

import com.console.mobile.core.util.UsageTone
import com.console.mobile.core.util.contextTone
import com.console.mobile.core.util.formatContextUsage
import com.console.mobile.core.util.formatTokens
import com.console.mobile.core.util.formatUsageAmount
import com.console.mobile.core.util.formatUsageValue
import com.console.mobile.core.util.limitTone
import com.console.mobile.core.util.ringTone
import com.console.mobile.core.util.usageResetLabel
import com.console.mobile.core.util.usedPercent
import console.v1.UsageAmount
import console.v1.UsageLimit
import console.v1.UsageWindow
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Mirrors desktop's usage_panel.rs / usage.rs so both clients print the same numbers. */
class UsageSheetFormatTest {
    private fun limit(
        used: Double? = null, cap: Double? = null, unit: String = "requests",
        usedFraction: Double? = null, status: String? = null, resetLabel: String? = null,
    ) = UsageLimit(
        id = "l", label = "Weekly", status = status,
        amount = UsageAmount(used = used, limit = cap, used_fraction = usedFraction, unit = unit),
        window = UsageWindow(reset_label = resetLabel),
    )

    @Test
    fun contextTileReadsLikeDesktop() {
        assertEquals("842/8.0k", formatContextUsage(842, 8000))
        assertEquals("12.4k/200.0k", formatContextUsage(12_400, 200_000))
        assertEquals("1.0M/2.0M", formatContextUsage(1_000_000, 2_000_000))
        assertEquals("0", formatTokens(0))
    }

    @Test
    fun tokenFormattingIgnoresTheDeviceLocale() {
        val before = java.util.Locale.getDefault()
        try {
            java.util.Locale.setDefault(java.util.Locale.GERMANY) // decimal comma
            assertEquals("1.5k", formatTokens(1_500))
            assertEquals("3.2h", formatUsageAmount(192.0, "minutes"))
        } finally {
            java.util.Locale.setDefault(before)
        }
    }

    @Test
    fun amountsPerUnit() {
        assertEquals("3.2h", formatUsageAmount(192.0, "minutes"))
        assertEquals("45m", formatUsageAmount(45.0, "minutes"))
        assertEquals("2.5K", formatUsageAmount(2500.0, "tokens"))
        assertEquals("1.2M", formatUsageAmount(1_200_000.0, "tokens"))
        assertEquals("812", formatUsageAmount(812.0, "tokens"))
        assertEquals("42%", formatUsageAmount(42.0, "percent"))
        assertEquals("$3.50", formatUsageAmount(3.5, "usd"))
        assertEquals("17", formatUsageAmount(17.0, "requests"))
        assertEquals("17", formatUsageAmount(17.0, "something-new"))
    }

    @Test
    fun limitValueShapes() {
        assertEquals("45m/5.0h", formatUsageValue(limit(used = 45.0, cap = 300.0, unit = "minutes"), 15.0))
        assertEquals("812 used", formatUsageValue(limit(used = 812.0, unit = "tokens"), 0.0))
        // A percent unit never prints used/limit.
        assertEquals("42%", formatUsageValue(limit(used = 42.0, cap = 100.0, unit = "percent"), 42.0))
        assertEquals("30%", formatUsageValue(UsageLimit(id = "l", label = "x"), 30.0))
    }

    @Test
    fun usedPercentFallbackOrder() {
        assertEquals(25.0, usedPercent(limit(usedFraction = 0.25)), 0.001)
        assertEquals(50.0, usedPercent(limit(used = 5.0, cap = 10.0)), 0.001)
        assertEquals(60.0, usedPercent(limit(used = 60.0, unit = "percent")), 0.001)
        assertEquals(0.0, usedPercent(UsageLimit(id = "l", label = "x")), 0.0)
        assertEquals(100.0, usedPercent(limit(usedFraction = 3.0)), 0.001) // clamped
    }

    @Test
    fun limitToneThresholds() {
        assertEquals(UsageTone.Normal, limitTone(limit(), 40.0))
        assertEquals(UsageTone.Warning, limitTone(limit(), 80.0))
        assertEquals(UsageTone.Danger, limitTone(limit(), 95.0))
        // The server's own status wins even at a low percent.
        assertEquals(UsageTone.Danger, limitTone(limit(status = "exhausted"), 10.0))
        assertEquals(UsageTone.Warning, limitTone(limit(status = "warning"), 10.0))
    }

    @Test
    fun contextToneUsesTheServerThreshold() {
        assertEquals(UsageTone.Normal, contextTone(30.0, 0.8))
        assertEquals(UsageTone.Warning, contextTone(60.0, 0.8))
        assertEquals(UsageTone.Danger, contextTone(80.0, 0.8))
        // Unset threshold must not read as "always over".
        assertEquals(UsageTone.Warning, contextTone(60.0, 0.0))
        assertEquals(UsageTone.Danger, contextTone(90.0, 0.0))
    }

    @Test
    fun resetLabelPrefersTheServersText() {
        assertEquals("resets Mon", usageResetLabel(limit(resetLabel = "resets Mon")))
        assertNull(usageResetLabel(limit()))
    }

    @Test
    fun ringMatchesDesktopThresholds() {
        assertEquals(UsageTone.Normal, ringTone(0.0))
        assertEquals(UsageTone.Normal, ringTone(79.9))
        assertEquals(UsageTone.Warning, ringTone(80.0))
        assertEquals(UsageTone.Warning, ringTone(94.9))
        assertEquals(UsageTone.Danger, ringTone(95.0))
        assertEquals(UsageTone.Danger, ringTone(100.0))
    }
}
