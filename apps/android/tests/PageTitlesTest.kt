package com.console.mobile

import com.console.mobile.core.util.bareWebUrls
import com.console.mobile.core.util.extractHtmlTitle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Display titles for bare web links. */
class PageTitlesTest {
    @Test
    fun readsATitleRegardlessOfCase() {
        assertEquals("Docs", extractHtmlTitle("<HEAD><TITLE data-x=\"1\">Docs</TITLE></HEAD>"))
    }

    @Test
    fun collapsesWhitespaceAndDecodesEntities() {
        assertEquals("Tom & Jerry – A\"s", extractHtmlTitle("<title>\n  Tom &amp; Jerry\n  &#8211; &#x41;&quot;s  </title>"))
    }

    @Test
    fun keepsUnknownEntitiesLiteral() {
        assertEquals("a &bogus; b", extractHtmlTitle("<title>a &bogus; b</title>"))
    }

    @Test
    fun missingOrEmptyTitlesAreNull() {
        assertNull(extractHtmlTitle("<html></html>"))
        assertNull(extractHtmlTitle("<title>   </title>"))
        assertNull(extractHtmlTitle("<title>unterminated"))
    }

    @Test
    fun longTitlesAreTruncated() {
        val title = extractHtmlTitle("<title>${"word ".repeat(40)}</title>")!!
        assertTrue(title.length <= 90)
        assertTrue(title.endsWith("…"))
    }

    @Test
    fun findsOnlyPublicBareUrls() {
        val urls = bareWebUrls("see https://example.com/a, and http://localhost:3000 plus https://docs.rs/x.")
        assertEquals(listOf("https://example.com/a", "https://docs.rs/x"), urls)
    }
}
