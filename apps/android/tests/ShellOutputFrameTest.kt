package com.console.mobile

import com.console.mobile.data.api.ConsoleJson
import com.console.mobile.data.model.BackgroundWorkOutput
import com.console.mobile.data.model.BackgroundWorkOutputFrame
import com.console.mobile.data.model.applyFrame
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Pushed shell-output frames (…/output/stream) fold into the accumulated output. */
class ShellOutputFrameTest {
    private fun frame(json: String) = ConsoleJson.decodeFromString<BackgroundWorkOutputFrame>(json)

    @Test
    fun framesAccumulateAndStderrOnlyReplacesWhenPresent() {
        var out: BackgroundWorkOutput? = null
        out = out.applyFrame(frame("""{"stdout":"one\n","nextCursor":4,"status":"running"}"""))
        out = out.applyFrame(frame("""{"stdout":"two\n","stderr":"warn","nextCursor":8,"status":"running"}"""))
        out = out.applyFrame(frame("""{"stdout":"","nextCursor":8,"status":"exited","exitCode":0}"""))

        assertEquals("one\ntwo\n", out.stdout)
        assertEquals("warn", out.stderr)
        assertEquals(8, out.nextCursor)
        assertEquals("exited", out.status)
        assertEquals(0, out.exitCode)
    }

    @Test
    fun absentStderrIsNullInTheFrameAndKeepsThePreviousValue() {
        assertNull(frame("""{"stdout":"x","status":"running"}""").stderr)
        var out: BackgroundWorkOutput? = null
        out = out.applyFrame(frame("""{"stderr":"first","status":"running"}"""))
        out = out.applyFrame(frame("""{"stdout":"x","status":"running"}"""))
        assertEquals("first", out.stderr)
    }

    @Test
    fun truncationSticks() {
        var out: BackgroundWorkOutput? = null
        out = out.applyFrame(frame("""{"stdout":"a","truncated":true,"status":"running"}"""))
        out = out.applyFrame(frame("""{"stdout":"b","truncated":false,"status":"running"}"""))
        assertTrue(out.truncated)
    }
}
