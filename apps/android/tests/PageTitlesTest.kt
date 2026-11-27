package com.console.mobile

import com.console.mobile.core.util.bareWebUrls
import com.console.mobile.core.util.extractHtmlTitleFallback
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Which bare links get a title lookup. Extracting the title from the fetched
 * HTML moved to `console-shared` (`page_title_test.rs`).
 */
class PageTitlesTest {
    @Test
    fun findsOnlyPublicBareUrls() {
        val urls = bareWebUrls("see https://example.com/a, and http://localhost:3000 plus https://docs.rs/x.")
        assertEquals(listOf("https://example.com/a", "https://docs.rs/x"), urls)
    }

    @Test
    fun fallbackExtractsTitleFromHtml() {
        assertEquals("My Page Title", extractHtmlTitleFallback("<html><head><title>My Page Title</title></head></html>"))
        assertEquals("Spaced Title", extractHtmlTitleFallback("<title>  Spaced\n Title  </title>"))
    }
}
