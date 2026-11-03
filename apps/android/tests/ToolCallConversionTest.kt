package com.console.mobile

import com.console.mobile.data.model.toUi
import okio.ByteString.Companion.encodeUtf8
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The server classifies every tool call (label, kind flags, icon key) and the
 * transcript rows render from those fields. Dropping them in the wire-to-UI
 * conversion is silent: the row falls back to the raw tool name and a generic
 * icon. These tests pin each field through the conversion.
 */
class ToolCallConversionTest {
    private fun wire(
        name: String = "bash",
        label: String? = null,
        flags: Int? = null,
        icon: String? = null,
    ) = console.v1.ToolCall(
        id = "c1", name = name, arguments = """{"command":"ls"}""".encodeUtf8(),
        label = label, kind_flags = flags, icon = icon,
    )

    @Test
    fun theServersLabelIconAndFlagsSurviveConversion() {
        val ui = wire(label = "Run Command", flags = 0, icon = "terminal").toUi()
        assertEquals("Run Command", ui.label)
        assertEquals("terminal", ui.icon)
        assertEquals(0, ui.kindFlags)
        assertEquals("Run Command", ui.displayLabel)
    }

    @Test
    fun everyKindFlagIsCarriedAcross() {
        val edit = wire("editFile", "Edit File", 1 or 8, "pencil").toUi()
        assertTrue(edit.isEdit); assertTrue(edit.isFileTarget); assertFalse(edit.isWrite); assertFalse(edit.isRead)

        val write = wire("write_file", "Write File", 2 or 8, "pencil").toUi()
        assertTrue(write.isWrite); assertTrue(write.isFileTarget); assertFalse(write.isEdit)

        val read = wire("read_file", "Read File", 4 or 8, "file").toUi()
        assertTrue(read.isRead); assertTrue(read.isFileTarget)

        val sub = wire("subagent", "Subagent", 16, null).toUi()
        assertTrue(sub.isSubagent); assertFalse(sub.isFileTarget)
    }

    @Test
    fun aCallWithNoClassificationFallsBackToTheRawName() {
        val ui = wire(name = "mystery").toUi()
        assertNull(ui.label)
        assertNull(ui.icon)
        assertEquals("mystery", ui.displayLabel)
        assertFalse(ui.isFileTarget)
    }

    @Test
    fun theCallsOwnFieldsAreNotLost() {
        val ui = wire(label = "Run Command", flags = 0, icon = "terminal").toUi()
        assertEquals("c1", ui.id)
        assertEquals("bash", ui.name)
        assertEquals("ls", ui.arguments?.toString()?.let { Regex("\"command\"\\s*:\\s*\"(.*?)\"").find(it)?.groupValues?.get(1) })
    }
}
