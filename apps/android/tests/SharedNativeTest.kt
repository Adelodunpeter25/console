package com.console.mobile

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A build without the staged Rust library must still run.
 *
 * `libconsole_wt_android.so` is gitignored and built by `rust-wt/build.sh`, so
 * a fresh clone has none, and on the JVM — where these tests run — it is never
 * present. That is the same condition as a stale or missing library on a
 * device: every function must answer with its documented inert value instead
 * of throwing out of `SharedNative`'s initialiser, which would take the app
 * down from `AppContainer.initialize`.
 *
 * What the Rust functions *return* is tested in Rust (`console-shared/tests`),
 * once, for both clients. This covers only the Kotlin side of the boundary.
 */
class SharedNativeTest {
    @Test
    fun theJvmRuntimeHasNoLibraryAndSaysSo() {
        assertFalse("the JVM test runtime has no Android .so", SharedNative.available)
    }

    @Test
    fun everyFunctionIsInertWithoutTheLibrary() {
        assertNull(SharedNative.pageTitle("<title>x</title>"))
        assertNull(SharedNative.mendMarkdown("**hi"))
        assertEquals("file", SharedNative.fileIcon("a.rs"))
        assertNull(SharedNative.languageIcon("rust"))
        assertNull(SharedNative.contextWindow(200_000))
        assertNull(SharedNative.modelName("gpt-5"))
        assertNull(SharedNative.tokensPerSecond(42.0))
        assertEquals("host", SharedNative.displayHost("host"))
        assertEquals(SharedNative.Diff.EMPTY, SharedNative.diff("a", "b"))
        assertEquals(SharedNative.Diff.EMPTY, SharedNative.parseDiff("@@ -1 +1 @@\n-a\n+b\n"))
        assertTrue(SharedNative.callDiffs("editFile", "{}").isEmpty())
    }

    @Test
    fun pairingFailsClosed() {
        assertTrue(SharedNative.pairing("http://h:1#v=1&server=s&code=c").isFailure)
    }

    @Test
    fun theAbiNumberIsPinnedToTheRustSide() {
        // Bump SharedNative.EXPECTED_ABI together with ABI_VERSION in
        // rust-wt/src/shared.rs; a mismatch makes `available` false on device.
        val rust = java.io.File("../rust-wt/src/shared.rs").takeIf { it.exists() }?.readText() ?: return
        val declared = Regex("""const ABI_VERSION: jint = (\d+);""").find(rust)?.groupValues?.get(1)?.toInt()
        assertEquals(declared, SharedNative.EXPECTED_ABI)
    }
}
