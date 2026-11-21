package com.console.mobile

import com.console.mobile.core.util.getToolIcon
import com.console.mobile.data.model.ToolCall
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Tool
import org.junit.Assert.assertEquals
import org.junit.Test

/** Icons come from the server's classification (console.v1 ToolCall.icon). */
class ToolHelpersTest {
    @Test
    fun serverIconKeyDrivesTheGlyph() {
        assertEquals(
            TablerIcons.Outline.PlugConnected,
            getToolIcon(ToolCall(id = "c1", name = "loadTools", icon = "package")),
        )
        assertEquals(
            TablerIcons.Outline.Tool,
            getToolIcon(ToolCall(id = "c2", name = "frobnicate")),
        )
    }
}
