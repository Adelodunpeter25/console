package com.console.mobile

import com.console.mobile.core.util.faviconHostOrNull
import com.console.mobile.core.util.faviconUrl
import com.console.mobile.core.util.filePathOrNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** Which paths and links get an inline file icon or favicon in markdown. */
class LinkIconsTest {
    @Test
    fun recognisesFilePaths() {
        assertEquals("apps/server/index.ts", filePathOrNull("apps/server/index.ts"))
        assertEquals("./foo.rs", filePathOrNull("./foo.rs"))
        assertEquals("README.md", filePathOrNull("README.md"))
        assertEquals("src/lib.rs", filePathOrNull("src/lib.rs:12:3").let { "src/lib.rs" })
        assertEquals("src/lib.rs", filePathOrNull("src/lib.rs:12:3"))
    }

    @Test
    fun leavesOrdinaryCodeAlone() {
        assertNull(filePathOrNull("1.2.3"))
        assertNull(filePathOrNull("e.g."))
        assertNull(filePathOrNull("foo()"))
        assertNull(filePathOrNull("let x = 1"))
        assertNull(filePathOrNull("src/"))
        assertNull(filePathOrNull("https://example.com/a.png"))
    }

    @Test
    fun faviconHostsSkipLocalTargets() {
        assertEquals("example.com", faviconHostOrNull("https://example.com/path?q=1"))
        assertEquals("docs.rs", faviconHostOrNull("http://user@docs.rs:8080/x"))
        assertEquals("www.rust-lang.org", faviconHostOrNull("www.rust-lang.org"))
        assertNull(faviconHostOrNull("http://localhost:3000"))
        assertNull(faviconHostOrNull("http://127.0.0.1:8080"))
        assertNull(faviconHostOrNull("mailto:a@b.com"))
        assertNull(faviconHostOrNull("src/main.rs"))
    }

    @Test
    fun faviconUrlUsesGoogleService() {
        assertEquals("https://www.google.com/s2/favicons?domain=example.com&sz=64", faviconUrl("example.com"))
    }
}
