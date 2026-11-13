//! Unit tests for chat tool-call row icons.
//!
//! Icons come from the server-classified icon key on each `ToolCall`
//! (tools.ToolIconKey, classify.go); a call without one falls back to the
//! generic wrench. These tests pin the key → glyph mapping.

use console_ui::primitives::{IconName, activity_icon};

#[test]
fn test_server_icon_keys_map_to_glyphs() {
    assert_eq!(activity_icon("search"), IconName::Search.path());
    assert_eq!(activity_icon("file"), IconName::File.path());
    assert_eq!(activity_icon("folder"), IconName::Folder.path());
    assert_eq!(activity_icon("terminal"), IconName::Terminal.path());
    assert_eq!(activity_icon("globe"), IconName::Globe.path());
    assert_eq!(activity_icon("pencil"), IconName::Pencil.path());
}

#[test]
fn test_every_entry_of_console_tool_icon_table() {
    for key in [
        "sparkle", "terminal", "pencil", "file", "folder", "search", "globe", "chart", "list",
        "memory", "package",
    ] {
        // Every mapped key must produce something other than the wrench.
        assert_ne!(activity_icon(key), IconName::Wrench.path(), "{key}");
    }
}
