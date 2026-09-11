package com.console.mobile.data.stream

import com.console.mobile.data.model.AgentSessionEvent
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runTest

/**
 * Controller lifecycle tests (no network): terminal frames, 409 fast-path,
 * reconnect backoff accounting, cancel inertness, finalize-at-most-once.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class RunStreamControllerTest {
    private class FakeDeps(val scope: TestScope) : RunStreamController.Deps {
        val events = mutableListOf<AgentSessionEvent>()
        val errors = mutableListOf<String>()
        var finalizeCount = 0
        var lastFinalizeError: Boolean? = null
        var attachCalls = 0

        override fun handleEvent(event: AgentSessionEvent) { events.add(event) }
        override fun markError(message: String) { errors.add(message) }
        override fun finalize(hadError: Boolean) {
            finalizeCount += 1
            lastFinalizeError = hadError
        }
        override fun baseUrl(): String = "http://localhost:3000"
        override fun authToken(): String? = null
        override fun runBodyJson(): String = "{\"prompt\":\"hi\"}"
        override fun scope(): kotlinx.coroutines.CoroutineScope = scope
    }

    private fun controller(scope: TestScope): Pair<RunStreamController, FakeDeps> {
        val deps = FakeDeps(scope)
        return RunStreamController("s1", deps) to deps
    }

    @Test fun doneFrameFinalizesOnce() = runTest {
        val (c, deps) = controller(this)
        c.onEvent(AgentSessionEvent(type = "turnStart", prompt = "hi"), seq = 1)
        assertEquals(1L, c.lastSeqValue)
        c.onEvent(AgentSessionEvent(type = "done"), seq = 2)
        c.onEvent(AgentSessionEvent(type = "done"), seq = 3) // late duplicate inert
        assertEquals(1, deps.finalizeCount)
        assertEquals(false, deps.lastFinalizeError)
        assertEquals(1, deps.events.size)
    }

    @Test fun errorEventMarksHadError() = runTest {
        val (c, deps) = controller(this)
        c.onEvent(
            AgentSessionEvent(type = "error", error = com.console.mobile.data.model.EventError("boom")),
            seq = null,
        )
        c.onEnd(false)
        assertEquals(1, deps.finalizeCount)
        assertEquals(true, deps.lastFinalizeError)
    }

    @Test fun abortErrorEventDoesNotMarkHadError() = runTest {
        val (c, deps) = controller(this)
        c.onEvent(
            AgentSessionEvent(type = "error", error = com.console.mobile.data.model.EventError("Run was aborted.")),
            seq = null,
        )
        c.onEnd(false)
        assertEquals(false, deps.lastFinalizeError)
    }

    @Test fun status409FinishesWithoutReconnect() = runTest {
        val (c, deps) = controller(this)
        c.onError("No active run", statusCode = 409)
        assertEquals(1, deps.finalizeCount)
        assertTrue(deps.errors.isEmpty())
    }

    @Test fun transportFailureRetriesThenSurfaces() = runTest {
        val (c, deps) = controller(this)
        // Attempts 1..3 schedule reconnects (end pairs are swallowed).
        repeat(3) {
            c.onError("drop", statusCode = null)
            c.onEnd(false)
        }
        assertEquals(0, deps.finalizeCount)
        assertTrue(deps.errors.isEmpty())
        // 4th failure exhausts retries -> surfaces error, then end finalizes.
        c.onError("drop", statusCode = null)
        assertEquals(listOf("drop"), deps.errors)
        c.onEnd(false)
        assertEquals(1, deps.finalizeCount)
        assertEquals(true, deps.lastFinalizeError)
    }

    @Test fun cancelMakesLateCallbacksInert() = runTest {
        val (c, deps) = controller(this)
        c.cancel()
        assertFalse(c.isActive)
        c.onEvent(AgentSessionEvent(type = "done"), seq = 1)
        c.onError("x", null)
        c.onEnd(false)
        assertEquals(0, deps.finalizeCount)
        assertTrue(deps.events.isEmpty())
    }
}
