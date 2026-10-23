//! Tool-call labels and classification.

use console_shared::tools::{
    is_edit_file, is_file_target_tool, is_read_file, is_subagent, is_write_file,
    parse_mcp_tool_name, tool_label,
};

#[test]
fn aliases_share_one_label() {
    assert_eq!(tool_label("readFile"), tool_label("read_file"));
    assert_eq!(tool_label("editFile"), "Edit File");
    assert_eq!(tool_label("str_replace"), "Edit File");
    assert_eq!(tool_label("listDir"), tool_label("list_dir"));
    assert_eq!(tool_label("webSearch"), tool_label("web_search"));
}

#[test]
fn every_server_tool_has_a_label() {
    // Names the Go server registers; a raw name here means the UI shows a
    // camelCase identifier instead of a sentence.
    for name in [
        "read_file", "readSkill", "list_dir", "glob", "grep", "write_file", "editFile",
        "batchWrite", "bash", "bashJob", "webSearch", "webFetch", "todo", "subagent", "ask",
        "askMany", "memory", "browser", "loadTools",
    ] {
        assert_ne!(tool_label(name), name, "{name} has no label");
    }
}

#[test]
fn mcp_tools_are_labelled_by_tool_and_server() {
    assert_eq!(
        tool_label("mcp__github__create_issue"),
        "create issue (github)"
    );
    assert_eq!(
        parse_mcp_tool_name("mcp__a-b__c_d"),
        Some(("a-b", "c_d"))
    );
}

#[test]
fn malformed_mcp_names_fall_back_to_the_raw_name() {
    for name in ["mcp__", "mcp____tool", "mcp__server__", "mcp__server", "other"] {
        assert_eq!(parse_mcp_tool_name(name), None, "{name}");
        assert_eq!(tool_label(name), name);
    }
}

#[test]
fn classification_predicates() {
    assert!(is_edit_file("edit_file") && !is_edit_file("writeFile"));
    assert!(is_write_file("batch_write") && !is_write_file("editFile"));
    assert!(is_read_file("Read") && is_read_file("view"));
    assert!(is_file_target_tool("read_file"));
    assert!(!is_file_target_tool("grep"), "a search scope is not a file");
    assert!(is_subagent("subagent") && !is_subagent("subagents"));
}
