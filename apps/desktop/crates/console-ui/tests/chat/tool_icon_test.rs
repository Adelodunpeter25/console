//! Unit tests for chat tool-call row icons.
//!
//! Single-call rows show the file's type icon only for file-targeting tools;
//! every other tool falls back to `activity_icon`, which must resolve both
//! the snake_case and camelCase spellings the agent runtime emits.

use console_ui::primitives::{IconName, activity_icon};

#[test]
fn test_search_tools_use_search_glyph() {
    let expected = IconName::Search.path();
    for name in ["grep", "search_files", "glob"] {
        assert_eq!(activity_icon(name), expected, "{name}");
    }
}

#[test]
fn test_camel_case_tools_match_snake_case_twins() {
    assert_eq!(activity_icon("readFile"), activity_icon("read_file"));
    assert_eq!(activity_icon("editFile"), activity_icon("edit_file"));
    assert_eq!(activity_icon("writeFile"), activity_icon("write_file"));
    assert_eq!(activity_icon("listDir"), activity_icon("list_files"));
    assert_eq!(activity_icon("webSearch"), activity_icon("web_search"));
}

#[test]
fn test_file_tools_keep_distinct_glyphs() {
    assert_eq!(activity_icon("readFile"), IconName::File.path());
    assert_eq!(activity_icon("listDir"), IconName::Folder.path());
}
