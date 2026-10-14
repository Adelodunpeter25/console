package com.console.mobile

import com.console.mobile.core.util.getToolIcon
import com.console.mobile.core.util.getToolLabel
import com.console.mobile.core.util.parseMcpToolName
import com.console.mobile.core.util.toolCallSummary
import com.console.mobile.data.model.ToolCall
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Tool
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * Labels, icons, and summaries for the loader + namespaced MCP tools.
 * `mcp__<server>__<tool>` names are dynamic, so they parse rather than map.
 */
class ToolHelpersTest {
    @Test
    fun loadToolsHasLabelIconAndGroupSummary() {
        assertEquals("Load Tools", getToolLabel("loadTools"))
        assertEquals(TablerIcons.Outline.PlugConnected, getToolIcon("loadTools"))
        val call = ToolCall(id = "c1", name = "loadTools", arguments = buildJsonObject { put("group", "mcp:github") })
        assertEquals("mcp:github", toolCallSummary(call))
    }

    @Test
    fun mcpNamesRenderAsToolServer() {
        assertEquals("create pr (github)", getToolLabel("mcp__github__create_pr"))
        assertEquals("my tool (my-server)", getToolLabel("mcp__my-server__my_tool"))
        assertEquals(TablerIcons.Outline.PlugConnected, getToolIcon("mcp__github__create_pr"))
    }

    @Test
    fun malformedMcpNamesFallBackToRaw() {
        assertEquals("mcp__", getToolLabel("mcp__"))
        assertEquals("mcp__onlyserver", getToolLabel("mcp__onlyserver"))
        assertEquals("mcp____", getToolLabel("mcp____"))
        assertNull(parseMcpToolName("mcp__"))
        assertNull(parseMcpToolName("mcp__onlyserver"))
        assertNull(parseMcpToolName("plain_tool"))
    }

    @Test
    fun unknownToolsKeepExistingFallbacks() {
        assertEquals("frobnicate", getToolLabel("frobnicate"))
        assertEquals(TablerIcons.Outline.Tool, getToolIcon("frobnicate"))
    }
}
