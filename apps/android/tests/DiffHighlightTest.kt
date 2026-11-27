package com.console.mobile

import com.console.mobile.ui.code.highlightCode
import com.console.mobile.ui.code.highlightCodeLine
import com.console.mobile.ui.code.isHighlightableLanguage
import com.console.mobile.ui.code.languageForPath
import com.console.mobile.ui.theme.ConsoleColors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class DiffHighlightTest {

    @Test
    fun isHighlightableLanguageFiltersProseAndRecognizesCode() {
        assertTrue(isHighlightableLanguage("kt"))
        assertTrue(isHighlightableLanguage("go"))
        assertTrue(isHighlightableLanguage("rs"))
        assertTrue(isHighlightableLanguage("ts"))
        assertTrue(isHighlightableLanguage("python"))
        assertTrue(isHighlightableLanguage("json"))
        assertTrue(isHighlightableLanguage("toml"))

        assertFalse(isHighlightableLanguage("md"))
        assertFalse(isHighlightableLanguage("markdown"))
        assertFalse(isHighlightableLanguage("txt"))
        assertFalse(isHighlightableLanguage("log"))
        assertFalse(isHighlightableLanguage(""))
        assertFalse(isHighlightableLanguage(null))
    }

    @Test
    fun doesNotHighlightMarkdownOrPlainText() {
        val line = "A requested policy is not the same thing as what a platform requires"
        val mdHighlighted = highlightCodeLine(line, "md")
        assertTrue("Markdown lines should not contain code token spans", mdHighlighted.spanStyles.isEmpty())
        assertEquals(line, mdHighlighted.text)

        val txtHighlighted = highlightCodeLine(line, "txt")
        assertTrue("Plain text lines should not contain code token spans", txtHighlighted.spanStyles.isEmpty())

        val noLangHighlighted = highlightCodeLine(line, "")
        assertTrue("Lines with no language should not contain code token spans", noLangHighlighted.spanStyles.isEmpty())
    }

    @Test
    fun languageForPathResolvesExtensionsAndSpecialNames() {
        assertEquals("kt", languageForPath("src/DiffView.kt"))
        assertEquals("rs", languageForPath("/path/to/main.rs"))
        assertEquals("ts", languageForPath("index.ts"))
        assertEquals("docker", languageForPath("Dockerfile"))
        assertEquals("make", languageForPath("Makefile"))
        assertEquals("", languageForPath("LICENSE"))
        assertEquals("", languageForPath(null))
        assertEquals("", languageForPath(""))
    }

    @Test
    fun emptyLineRendersSpaceForGutterAlignment() {
        val highlighted = highlightCodeLine("", "kt")
        assertEquals(" ", highlighted.text)
    }

    @Test
    fun highlightsKeywordsAndNumbers() {
        val highlighted = highlightCodeLine("val count = 42", "kt")
        val spans = highlighted.spanStyles
        // "val" keyword span
        val valSpan = spans.firstOrNull { it.start == 0 && it.end == 3 }
        assertTrue("Expected keyword span for 'val'", valSpan != null)
        assertEquals(ConsoleColors.Syntax.Keyword, valSpan?.item?.color)

        // "42" number span
        val numSpan = spans.firstOrNull { it.start == 12 && it.end == 14 }
        assertTrue("Expected number span for '42'", numSpan != null)
        assertEquals(ConsoleColors.Syntax.Number, numSpan?.item?.color)
    }

    @Test
    fun highlightsCommentsAndStrings() {
        val commentLine = highlightCodeLine("// this is a comment", "kt")
        val commentSpan = commentLine.spanStyles.firstOrNull { it.start == 0 && it.end == 20 }
        assertTrue("Expected comment span", commentSpan != null)
        assertEquals(ConsoleColors.Syntax.Comment, commentSpan?.item?.color)

        val stringLine = highlightCodeLine("val msg = \"hello world\"", "kt")
        val stringSpan = stringLine.spanStyles.firstOrNull { it.start == 10 && it.end == 23 }
        assertTrue("Expected string span", stringSpan != null)
        assertEquals(ConsoleColors.Syntax.String, stringSpan?.item?.color)
    }

    @Test
    fun highlightsMultiLineCode() {
        val code = "val a = 1\nval b = 2"
        val highlighted = highlightCode(code, "kt")
        assertEquals(code, highlighted.text)
        assertTrue(highlighted.spanStyles.size >= 4)
    }
}
