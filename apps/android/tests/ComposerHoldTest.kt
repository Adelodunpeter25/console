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

/** The composer opens in step with the keyboard's own animated height. */
class ImeExpansionTest {
    @Test
    fun closedKeyboardMeansCollapsed() {
        assertEquals(0f, com.console.mobile.feature.chat.imeExpansion(0, 800), 0f)
    }

    @Test
    fun followsTheKeyboardAsItRisesAndFalls() {
        val f = { ime: Int, ref: Int -> com.console.mobile.feature.chat.imeExpansion(ime, ref) }
        assertEquals(0.25f, f(200, 800), 0.001f)
        assertEquals(0.5f, f(400, 800), 0.001f)
        // Same mapping going down, so closing mirrors opening.
        assertEquals(0.25f, f(200, 800), 0.001f)
    }

    @Test
    fun fullyOpenIsClampedToOne() {
        assertEquals(1f, com.console.mobile.feature.chat.imeExpansion(800, 800), 0f)
        assertEquals(1f, com.console.mobile.feature.chat.imeExpansion(1200, 800), 0f)
    }

    @Test
    fun missingReferenceCannotDivideByZero() {
        assertEquals(0f, com.console.mobile.feature.chat.imeExpansion(300, 0), 0f)
    }
}
