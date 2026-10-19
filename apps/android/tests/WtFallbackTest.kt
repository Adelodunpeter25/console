package com.console.mobile

import com.console.mobile.data.stream.WtTransport
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * A build without the staged Rust library must still run.
 *
 * `libconsole_wt_android.so` is gitignored and produced by `rust-wt/build.sh`,
 * so CI and fresh clones ship without it — and on the JVM, which is where these
 * tests run, it is never present. That is the same condition: every WebTransport
 * entry point must decline and let the SSE/WS path answer, instead of throwing
 * out of `WtNative`'s initializer and taking the app down during
 * `AppContainer.initialize`.
 *
 * The failing case is narrow and easy to miss: WebTransport is only attempted
 * on a TLS backend, so a plain-http dev server never touches the library and
 * hides the bug until someone points the app at an https URL.
 */
class WtFallbackTest {
    @Test
    fun reportsTheLibraryAsMissingInsteadOfThrowing() {
        assertFalse("the JVM test runtime has no Android .so", WtNative.available)
        assertEquals("WebTransport unavailable on this build", WtNative.describe(0L))
    }

    @Test(timeout = 15_000)
    fun tlsBackendFallsBackToTheSsePath() {
        // Both of these are reached from startup with the user's configured
        // (https) backend: notifications() and the chat stream.
        assertNull(
            WtTransport.watch(
                baseUrl = "https://dev.example.com",
                requestJson = WtTransport.watchRequest(WtTransport.TARGET_NOTIFICATIONS),
                frameType = WtTransport.FRAME_NOTIFICATION,
            ) { null },
        )
        assertNull(
            WtTransport.stream(
                baseUrl = "https://dev.example.com",
                sessionId = "s-1",
                requestType = WtStream.REQUEST_START,
                requestJson = "{}",
            ),
        )

        // reset() is called whenever the backend URL changes.
        WtTransport.reset()
    }
}
