package com.console.mobile

import com.console.mobile.feature.browser.isSecureUrl
import com.console.mobile.feature.browser.isWebUrl
import com.console.mobile.feature.browser.normalizeBrowserUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Address-bar normalization. Forwarded dev ports are typed as `host:port`, so
 * the browser has to turn those into http URLs while leaving real URLs, and
 * plain words, alone.
 */
class BrowserUrlTest {
    @Test
    fun schemeLessDevServerGetsHttp() {
        assertEquals("http://localhost:5173", normalizeBrowserUrl("localhost:5173"))
        assertEquals("http://10.0.2.2:3000/", normalizeBrowserUrl(" 10.0.2.2:3000/ "))
        assertEquals("http://192.168.1.20", normalizeBrowserUrl("192.168.1.20"))
        assertEquals("http://127.0.0.1:8080/app?x=1", normalizeBrowserUrl("127.0.0.1:8080/app?x=1"))
    }

    @Test
    fun explicitSchemeIsUntouched() {
        assertEquals("https://example.com/a", normalizeBrowserUrl("https://example.com/a"))
        assertEquals("about:blank", normalizeBrowserUrl("about:blank"))
    }

    @Test
    fun freeTextIsNotAnAddress() {
        assertNull(normalizeBrowserUrl("hello world"))
        assertNull(normalizeBrowserUrl("notaurl"))
        assertNull(normalizeBrowserUrl(""))
    }

    @Test
    fun webSchemesStayInTheWebView() {
        assertTrue(isWebUrl("https://example.com"))
        assertTrue(isWebUrl("http://127.0.0.1:5173/"))
        assertTrue(isWebUrl("about:blank"))
        assertFalse(isWebUrl("mailto:dev@example.com"))
        assertFalse(isWebUrl("intent://scan/#Intent;scheme=zxing;end"))
    }

    @Test
    fun lockIsShownOnlyForSecurePages() {
        assertTrue(isSecureUrl("https://example.com"))
        assertFalse(isSecureUrl("http://127.0.0.1:5173/"))
    }
}
