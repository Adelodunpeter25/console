use console_proto::{SessionDeleteResponse, SessionHeader};
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
fn decodes_golden_header_fixture() {
    let msg: SessionHeader =
        serde_json::from_str(fixture("header.json").trim()).expect("decode header");
    assert_eq!(msg.id, "s1");
    assert_eq!(msg.model_id, "gpt-5");
    assert_eq!(msg.project_id.as_deref(), Some("p1"));
    assert_eq!(msg.thinking_level.as_deref(), Some("medium"));
    // Timestamps arrive as protojson strings, not numbers.
    assert_eq!(msg.created_at, 1700000000000);
    assert_eq!(msg.updated_at, 1700000000001);
    assert_eq!(msg.message_count, Some(3));
    assert_eq!(msg.status, "done");
    let worktree = msg.worktree.as_ref().expect("worktree present");
    assert_eq!(worktree.branch, "feat");
}

#[test]
fn decodes_golden_list_fixture() {
    let items: Vec<SessionHeader> =
        serde_json::from_str(fixture("list.json").trim()).expect("decode list");
    assert_eq!(items.len(), 1);
    assert_eq!(items[0].id, "s1");
}

#[test]
fn decodes_golden_delete_fixtures() {
    let deleted: SessionDeleteResponse =
        serde_json::from_str(fixture("delete.json").trim()).expect("delete");
    assert_eq!(deleted.id, "s1");
    assert!(deleted.deleted);
    let restored: console_proto::SessionRestoreResponse =
        serde_json::from_str(fixture("restore.json").trim()).expect("restore");
    assert!(restored.restored);
}

#[test]
fn ignores_unknown_fields() {
    let msg: SessionHeader = serde_json::from_str(
        r#"{"id":"s","title":"t","cwd":"/","modelId":"m","provider":"p","approvalMode":"a","createdAt":"1","updatedAt":"2","status":"done","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(msg.id, "s");
}

#[test]
fn serializes_camel_case_strings() {
    let raw = serde_json::to_string(&SessionHeader {
        id: "s1".into(),
        status: "working".into(),
        ..Default::default()
    })
    .expect("encode header");
    assert!(!raw.contains("messageCount"), "unset fields must omit: {raw}");
}
