package com.console.mobile

import com.console.mobile.core.util.bareWebUrls
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
}
