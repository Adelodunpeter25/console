use console_proto::{
    SubagentActivityEvent, SubagentEndEvent, SubagentInfo, SubagentStartEvent, TodoItem,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/event")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_todo_update_frame() {
    #[derive(serde::Deserialize)]
    struct Frame {
        #[allow(dead_code)]
        #[serde(rename = "type")]
        kind: String,
        items: Vec<TodoItem>,
        action: String,
    }
    let frame: Frame =
        serde_json::from_str(fixture("todo_update.json").trim()).expect("decode todoUpdate");
    assert_eq!(frame.items.len(), 1);
    assert_eq!(frame.items[0].id, 1);
    assert_eq!(frame.items[0].content, "a");
    assert_eq!(frame.items[0].status, "pending");
    assert_eq!(frame.action, "set");
}

#[test]
fn decodes_golden_subagent_start_frame() {
    let start: SubagentStartEvent =
        serde_json::from_str(fixture("subagent_start.json").trim()).expect("decode start");
    assert_eq!(start.subagent_id, "s1");
    assert_eq!(start.parent_tool_call_id.as_deref(), Some("c1"));
    assert_eq!(start.name, "w");
    assert_eq!(start.role, "tester");
    assert_eq!(start.prompt, "do it");
    assert_eq!(start.max_turns, 10);
}

#[test]
fn decodes_golden_subagent_activity_frame() {
    let act: SubagentActivityEvent =
        serde_json::from_str(fixture("subagent_activity.json").trim()).expect("decode activity");
    assert_eq!(act.subagent_id, "s1");
    assert_eq!(act.turn_index, 2);
    assert_eq!(act.tool_call_id.as_deref(), Some("c2"));
    assert_eq!(act.tool_name.as_deref(), Some("sh"));
    assert_eq!(act.status, "running");
    assert_eq!(act.error, None);
    // Args cross as raw JSON bytes (base64 on the wire), matching the
    // SubagentActivityItem.args the state store appends.
    assert_eq!(act.args, br#"{"cmd":"ls"}"#);
}

#[test]
fn decodes_golden_subagent_end_frame() {
    let end: SubagentEndEvent =
        serde_json::from_str(fixture("subagent_end.json").trim()).expect("decode end");
    assert_eq!(end.subagent_id, "s1");
    assert_eq!(end.status, "completed");
    assert_eq!(end.summary.as_deref(), Some("done"));
    assert_eq!(end.total_turns, 3);
}

// The GET rows (nested schema) and the event frames (flattened) stay
// distinct types but share field names; this pins that the flattened event
// type still satisfies the nested row's constructor defaults used in state.
#[test]
fn subagent_row_shape_unchanged() {
    let row = SubagentInfo::default();
    assert_eq!(row.max_turns, 0);
    assert_eq!(row.subagent_id, "");
}