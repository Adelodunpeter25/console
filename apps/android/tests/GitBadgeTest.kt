package com.console.mobile

import com.console.mobile.core.util.gitBadge
import com.console.mobile.core.util.gitBadges
import com.console.mobile.core.util.gitRoleColor
import com.console.mobile.core.util.statusLetter
import com.console.mobile.ui.theme.NewTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Git badges on the phone.
 *
 * The rules themselves live in `console-shared` and reach the app over JNI,
 * so they are pinned in `console-shared/tests/git_badges_test.rs`. What is
 * tested here is the phone's own part: the palette a role is drawn in, and
 * the inert answer when the shared library is not there.
 */
class GitBadgeTest {
    @Test
    fun aRoleIsDrawnInTheAppsOwnPalette() {
        assertEquals(NewTheme.Success, gitRoleColor(SharedNative.GitRole.SUCCESS))
        assertEquals(NewTheme.Warning, gitRoleColor(SharedNative.GitRole.WARNING))
        assertEquals(NewTheme.Danger, gitRoleColor(SharedNative.GitRole.DANGER))
        assertEquals(NewTheme.TextSecondary, gitRoleColor(SharedNative.GitRole.MUTED))
    }

    @Test
    fun withoutTheSharedLibraryThereAreNoBadges() {
        // The library is optional at build time: a fresh clone, a JVM unit test
        // or a 32-bit ABI has none. The app must then draw no badge rather than
        // a guess that could contradict the desktop.
        assertFalse(SharedNative.available)
        assertNull(gitBadge("M"))
        assertTrue(gitBadges(emptyList(), emptyList()).isEmpty())
    }

    @Test
    fun withoutTheSharedLibraryTheLetterFallsBackToModified() {
        // The Changes screen keeps its layout whatever the library does, so an
        // unknown status still gets a letter in the width it reserved.
        assertEquals("M", statusLetter("M"))
    }

    @Test
    fun mismatchedPathAndStatusListsAreIgnored() {
        // A pairing mistake must not silently badge the wrong file.
        assertTrue(gitBadges(listOf("/a"), emptyList()).isEmpty())
    }
}
