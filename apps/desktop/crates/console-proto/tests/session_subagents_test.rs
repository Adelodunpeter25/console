use console_proto::{SubagentActivityItem, SubagentInfo};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/session")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_subagents_fixture() {
    let items: Vec<SubagentInfo> =
        serde_json::from_str(fixture("subagents.json").trim()).expect("decode subagents");
    assert_eq!(items.len(), 1);
    let sub = &items[0];
    assert_eq!(sub.subagent_id, "sub1");
    assert_eq!(sub.max_turns, 5);
    assert_eq!(sub.current_turn, 2);
    assert_eq!(sub.created_at, 1700000000000);
    assert_eq!(sub.activities.len(), 1);
    let act = &sub.activities[0];
    assert_eq!(act.turn_index, 2);
    assert_eq!(act.tool_call_id, "c9");
    let args: serde_json::Value =
        serde_json::from_slice(&act.args).expect("args json");
    assert_eq!(args["path"], "/x");
}

#[test]
fn ignores_unknown_fields() {
    let items: Vec<SubagentInfo> = serde_json::from_str(
        r#"[{"subagentId":"s","parentToolCallId":"c","name":"n","role":"r","prompt":"p","maxTurns":1,"currentTurn":1,"status":"running","createdAt":"1","updatedAt":"2","futureField":1}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(items[0].name, "n");
}

#[test]
fn serializes_counts_as_numbers() {
    // int32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
    let raw = serde_json::to_string(&SubagentActivityItem {
        turn_index: 2,
        tool_call_id: "c9".into(),
        tool_name: "read".into(),
        status: "completed".into(),
        ..Default::default()
    })
    .expect("encode activity");
    assert!(raw.contains(r#""turnIndex":2"#), "unexpected encoding: {raw}");
}
