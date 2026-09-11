package com.console.mobile.data.api

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class ApiEnvelopeTest {
    @Test fun unwrapOk() {
        assertEquals("v", unwrapEnvelope(Envelope(true, "v", null), "x"))
    }

    @Test fun unwrapThrows() {
        assertFailsWith<ApiException> { unwrapEnvelope<String>(Envelope(false, null, "nope"), "load") }
    }

    @Test fun sseFrames() {
        val (frames, rest) = extractSseFrames("event: gitStatus\ndata: {\"a\":1}\n\npartial")
        assertEquals(1, frames.size)
        assertEquals("gitStatus", frames[0].event)
        assertTrue(frames[0].data.contains("a"))
        assertEquals("partial", rest)
    }

    @Test fun runStreamPath() {
        assertEquals("/api/sessions/s/run/stream", getRunStreamPath("s"))
        assertEquals("/api/sessions/s/run/stream?since=5", getRunStreamPath("s", 5))
    }
}
