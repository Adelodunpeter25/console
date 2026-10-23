package com.console.mobile

import com.console.mobile.core.util.getToolIcon
import com.console.mobile.core.util.toolCallSummary
import com.console.mobile.data.model.ToolCall
import io.github.lyxnx.compose.ui.tablericons.TablerIcons
import io.github.lyxnx.compose.ui.tablericons.outline.PlugConnected
import io.github.lyxnx.compose.ui.tablericons.outline.Tool
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Icons and summaries for the loader + namespaced MCP tools.
 *
 * Labels and `mcp__<server>__<tool>` parsing moved to `console-shared`
 * (`tools_test.rs`), which both clients now use; what stays here is the
 * Compose-specific half.
 */
class ToolHelpersTest {
    @Test
    fun loadToolsHasLabelIconAndGroupSummary() {
        assertEquals(TablerIcons.Outline.PlugConnected, getToolIcon("loadTools"))
        val call = ToolCall(id = "c1", name = "loadTools", arguments = buildJsonObject { put("group", "mcp:github") })
        assertEquals("mcp:github", toolCallSummary(call))
    }

    @Test
    fun mcpToolsShareThePlugGlyph() {
        assertEquals(TablerIcons.Outline.PlugConnected, getToolIcon("mcp__github__create_pr"))
    }

    @Test
    fun unknownToolsKeepExistingFallbacks() {
        assertEquals(TablerIcons.Outline.Tool, getToolIcon("frobnicate"))
    }
}
