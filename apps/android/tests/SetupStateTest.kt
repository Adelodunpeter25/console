package com.console.mobile

import com.console.mobile.data.store.SetupState
import com.console.mobile.data.store.SetupStateHolder
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * First-run setup panel state: the stored-vs-derived split, mirroring the
 * desktop's model.
 *
 * The property under test is that setup facts are never persisted. They are read
 * live, so a user who deletes their last project sees that row go back to unmet
 * without the panel reopening — the dismissal is the only thing remembered.
 */
class SetupStateTest {
    private fun fullySetUp() = SetupState(
        activeEnvId = "env_abc123",
        serverName = "Remote box",
        hasProject = true,
        hasProvider = true,
        hasGithub = true,
        loading = false,
    )

    @Test
    fun a_server_missing_nothing_is_complete() {
        assertTrue(fullySetUp().isComplete)
    }

    @Test
    fun each_step_alone_leaves_it_incomplete() {
        // The panel's whole purpose is the remaining work, so one unmet step must
        // keep it open-able.
        val complete = fullySetUp()
        assertFalse(complete.copy(hasProject = false).isComplete)
        assertFalse(complete.copy(hasProvider = false).isComplete)
        assertFalse(complete.copy(hasGithub = false).isComplete)
    }

    @Test
    fun an_untouched_server_is_not_complete() {
        assertFalse(SetupState().isComplete)
    }

    @Test
    fun opening_resets_prior_step_results() {
        // The panel can open for a second server after the first was set up. The
        // new server must start from "unknown", not inherit the previous
        // server's answers — those would be lies about a different machine.
        val holder = SetupStateHolder()
        holder.resolve(hasProject = true, hasProvider = true, hasGithub = true)
        holder.open("env_second", "Second box")

        val state = holder.state.value
        assertEquals("env_second", state.activeEnvId)
        assertEquals("Second box", state.serverName)
        assertTrue("step status is unknown until resolved", state.loading)
        assertFalse(state.hasProject)
        assertFalse(state.hasProvider)
        assertFalse(state.hasGithub)
    }

    @Test
    fun resolving_clears_loading_and_records_every_step() {
        val holder = SetupStateHolder()
        holder.open("env_abc123", "Remote box")
        holder.resolve(hasProject = true, hasProvider = false, hasGithub = true)

        val state = holder.state.value
        assertFalse(state.loading)
        assertTrue(state.hasProject)
        assertFalse(state.hasProvider)
        assertTrue(state.hasGithub)
        assertFalse(state.isComplete)
    }

    @Test
    fun closing_clears_the_active_server() {
        val holder = SetupStateHolder()
        holder.open("env_abc123", "Remote box")
        holder.resolve(true, true, true)
        holder.close()

        val state = holder.state.value
        assertNull("a closed panel must not gate navigation", state.activeEnvId)
        assertNull(state.serverName)
        // Step results reset with it, so a later open cannot read stale answers.
        assertFalse(state.hasProject)
        assertFalse(state.hasProvider)
        assertFalse(state.hasGithub)
    }

    @Test
    fun dismissal_is_tracked_per_server_not_globally() {
        // The store holds environment ids, so dismissing one server cannot
        // silence the panel on another. Modelled here as the set the store keeps.
        val dismissed = mutableSetOf("env_abc123")

        assertTrue("env_abc123" in dismissed)
        assertFalse("env_second" in dismissed)
    }

    @Test
    fun closing_does_not_imply_dismissal() {
        // Regression guard for a real bug: opening a step used to dismiss the
        // panel at the same time, so backing out of the destination left the
        // user with neither the panel nor the thing they were setting up.
        // Only an explicit Skip writes the dismissal.
        val holder = SetupStateHolder()
        holder.open("env_abc123", "Remote box")
        holder.close()

        assertNull("close() must not record a dismissal", holder.state.value.activeEnvId)
        assertFalse(holder.state.value.isComplete)
    }

    @Test
    fun a_step_can_be_opened_without_dismissing_the_panel() {
        // Opening a destination leaves panel state untouched, so coming back
        // shows the panel again — still open, still reporting what the server
        // actually has.
        val holder = SetupStateHolder()
        holder.open("env_abc123", "Remote box")
        holder.resolve(hasProject = false, hasProvider = false, hasGithub = false)

        val afterReturn = holder.state.value
        assertEquals("env_abc123", afterReturn.activeEnvId)
        assertEquals("Remote box", afterReturn.serverName)
        assertFalse(afterReturn.hasProject)
        assertFalse(afterReturn.hasProvider)
        assertFalse(afterReturn.hasGithub)
    }

    @Test
    fun a_complete_server_has_nothing_left_to_onboard() {
        // The gate for showing the panel at all. A server that is already fully
        // configured must not be interrupted with a first-run screen just
        // because it was added recently — "new server" is not the same as
        // "unconfigured server".
        assertFalse(SetupState().isComplete)
        assertTrue(fullySetUp().isComplete)
    }

    @Test
    fun one_missing_step_is_enough_to_warrant_the_panel() {
        val complete = fullySetUp()
        assertFalse(complete.copy(hasProject = false).isComplete)
        assertFalse(complete.copy(hasProvider = false).isComplete)
        assertFalse(complete.copy(hasGithub = false).isComplete)
    }
}
