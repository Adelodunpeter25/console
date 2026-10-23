package com.console.mobile

import com.console.mobile.SharedNative
import com.console.mobile.core.util.FILE_LINK_PREFIX
import com.console.mobile.core.util.faviconHostOrNull
import com.console.mobile.core.util.fileLinkTarget
import com.console.mobile.core.util.filePathFromLink
import com.console.mobile.core.util.resolveFilePath
import com.console.mobile.core.util.faviconUrl
import com.console.mobile.core.util.filePathOrNull
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Which paths and links get an inline file icon or favicon in markdown. */
class LinkIconsTest {
    @Test
    fun recognisesFilePaths() {
        // "known file type" comes from the shared icon table, which lives in the
        // native library; a JVM run has none (Rust `icons_test` covers the table).
        assumeTrue(SharedNative.available)
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

    @Test
    fun fileLinksRoundTripThroughTheLinkScheme() {
        assertEquals("apps/a/b.kt", fileLinkTarget("apps/a/b.kt:12"))
        assertEquals("/tmp/x.md", fileLinkTarget("file:///tmp/x.md"))
        assertNull(fileLinkTarget("https://example.com/a.md"))
        assertNull(fileLinkTarget("mailto:a@b.com"))
        assertEquals("src/lib.rs", filePathFromLink(FILE_LINK_PREFIX + "src/lib.rs"))
        assertNull(filePathFromLink("https://example.com"))
    }

    @Test
    fun relativePathsResolveAgainstTheProjectRoot() {
        assertEquals("/proj/apps/a.kt", resolveFilePath("apps/a.kt", "/proj/"))
        assertEquals("/proj/a.kt", resolveFilePath("./a.kt:3", "/proj"))
        assertEquals("/abs/a.kt", resolveFilePath("/abs/a.kt", "/proj"))
        assertEquals("a.kt", resolveFilePath("a.kt", null))
    }

    @Test
    fun extensionlessNamesAreFilesToo() {
        assumeTrue(SharedNative.available)
        assertEquals("Makefile", filePathOrNull("Makefile"))
        assertEquals("LICENSE", filePathOrNull("LICENSE"))
        assertNull(filePathOrNull("hello"))
    }
}
