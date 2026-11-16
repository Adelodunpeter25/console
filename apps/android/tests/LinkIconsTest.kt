package com.console.mobile

import com.console.mobile.SharedNative
import com.console.mobile.core.util.BARE_FILE_PATH_REGEX
import com.console.mobile.core.util.FILE_LINK_PREFIX
import com.console.mobile.core.util.closestMatch
import com.console.mobile.core.util.isBareFileName
import com.console.mobile.core.util.parseLineSuffix
import com.console.mobile.core.util.withLineSuffix
import com.console.mobile.core.util.isBareFilePathMatch
import com.console.mobile.core.util.faviconHostOrNull
import com.console.mobile.core.util.fileLinkTarget
import com.console.mobile.core.util.filePathFromLink
import com.console.mobile.core.util.resolveFilePath
import com.console.mobile.core.util.faviconUrl
import com.console.mobile.core.util.filePathOrNull
import com.console.mobile.feature.chat.markdown.MdBlock
import com.console.mobile.feature.chat.markdown.parseBlocks
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assume.assumeTrue
import org.junit.Test

/** Which paths and links get an inline file icon or favicon in markdown. */
class LinkIconsTest {
    @Test
    fun recognisesFilePaths() {
        assertEquals("~/Downloads", filePathOrNull("~/Downloads"))
        assertEquals("~/Downloads/file.txt", filePathOrNull("~/Downloads/file.txt"))
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

    @Test
    fun bareWebUrlsInMarkdownTextGetFavicons() {
        val blocks = parseBlocks("Here is a link https://docs.rs/gpui and some text.")
        val p = blocks.first() as MdBlock.Paragraph
        val icons = p.text.getStringAnnotations("icon", 0, p.text.length)
        assertEquals(1, icons.size)
        assertEquals("favicon:docs.rs", icons.first().item)

        val links = p.text.getLinkAnnotations(0, p.text.length)
        assertEquals(1, links.size)
        val link = links.first().item as androidx.compose.ui.text.LinkAnnotation.Url
        assertEquals("https://docs.rs/gpui", link.url)
    }

    @Test
    fun formatsCiteQuoteForMobile() {
        assertEquals("> hello\n\n", com.console.mobile.feature.chat.markdown.formatCiteQuote("hello"))
        assertEquals("> first\n> second\n\n", com.console.mobile.feature.chat.markdown.formatCiteQuote("first\nsecond"))
        assertEquals("", com.console.mobile.feature.chat.markdown.formatCiteQuote("   "))
    }

    @Test
    fun barePathsInProseMatchDesktop() {
        fun found(text: String) = BARE_FILE_PATH_REGEX.findAll(text)
            .filter { isBareFilePathMatch(text, it.range.first, it.value) }.map { it.value }.toList()
        assertEquals(listOf("apps/server/index.ts"), found("see apps/server/index.ts now"))
        assertEquals(listOf("./src/foo.rs:12:3"), found("at ./src/foo.rs:12:3."))
        assertEquals(listOf("/abs/path/file.md"), found("(/abs/path/file.md)"))
        assertEquals(listOf("~/proj/file.png"), found("open ~/proj/file.png"))
        assertEquals(emptyList<String>(), found("costs \$0.0005/audio-minute"))
        assertEquals(emptyList<String>(), found("version 1.2.3 and e.g. foo()"))
    }

    @Test
    fun fileUrlsAreDecoded() {
        assertEquals("/tmp/example file.txt", fileLinkTarget("file:///tmp/example%20file.txt"))
        assertEquals("/tmp/a.rs", fileLinkTarget("file://localhost/tmp/a.rs:4:2"))
        assertEquals("/tmp/a b.rs", resolveFilePath("file:///tmp/a%20b.rs", "/root"))
    }

    @Test
    fun closestMatchPrefersNearestToCwd() {
        val c = listOf("/p/a/x/main.rs", "/p/b/main.rs", "/p/b/deep/main.rs")
        assertEquals("/p/b/main.rs", closestMatch("/p/b", c))
        assertEquals("/p/a/x/main.rs", closestMatch("/p/a/x", c))
        assertEquals(true, isBareFileName("main.rs:3"))
        assertEquals(false, isBareFileName("src/main.rs"))
    }

    @Test
    fun lineSuffixSurvivesTheLink() {
        assertEquals(12, parseLineSuffix("src/lib.rs:12:3"))
        assertEquals(null, parseLineSuffix("src/lib.rs"))
        assertEquals("/a/b.rs:4:2", withLineSuffix("/a/b.rs", "file:///a/b.rs:4:2"))
        assertEquals("src/lib.rs", resolveFilePath("src/lib.rs:12", null))
    }
}
