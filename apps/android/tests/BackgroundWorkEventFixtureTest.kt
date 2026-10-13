package com.console.mobile

import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.AgentSessionEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test

class BackgroundWorkEventFixtureTest {
    @Test
    fun decodesShellCompletionWithFlatWorkRecord() {
        val event = ConsoleJson.decodeFromString<AgentSessionEvent>(
            """{"type":"backgroundWorkCompleted","work":{"id":"work_test","sessionId":"session-1","kind":"shell","status":"completed","createdAt":"2026-10-13T09:45:44.936584122Z","metadata":{"job_id":"job_test","exit_code":"0"}}}""",
        )
        assertEquals("backgroundWorkCompleted", event.type)
        assertEquals("work_test", event.work?.id)
        assertEquals("job_test", event.work?.metadata?.get("job_id"))
        assertNotNull(event.work)
    }
}
