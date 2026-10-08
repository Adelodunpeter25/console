package com.console.mobile

import com.console.mobile.feature.chat.ComposerHold
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The composer folds to a pill on losing focus, but a sheet or the image picker
 * it launched also steals focus. The hold is what stops it folding up under them.
 */
class ComposerHoldTest {
    @Test
    fun heldWhileAnythingIsOpen() {
        val hold = ComposerHold()
        assertFalse(hold.held)
        hold.acquire()
        assertTrue(hold.held)
        hold.release()
        assertFalse(hold.held)
    }

    @Test
    fun staysHeldUntilEveryHolderReleases() {
        // A sheet opening the same moment the picker returns must not drop the hold early.
        val hold = ComposerHold()
        hold.acquire()
        hold.acquire()
        hold.release()
        assertTrue(hold.held)
        hold.release()
        assertFalse(hold.held)
    }

    @Test
    fun strayReleaseCannotGoNegative() {
        // A cancelled picker reports back once; an extra release must not
        // leave the next acquire "already released".
        val hold = ComposerHold()
        hold.release()
        hold.release()
        hold.acquire()
        assertTrue(hold.held)
        hold.release()
        assertFalse(hold.held)
        assertEquals(false, hold.held)
    }
}
