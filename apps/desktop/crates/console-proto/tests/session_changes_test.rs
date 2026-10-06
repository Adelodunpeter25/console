use console_proto::{SessionFileChange, SessionFileChangeDiff};
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
fn decodes_golden_changes_fixture() {
    let items: Vec<SessionFileChange> =
        serde_json::from_str(fixture("changes.json").trim()).expect("decode changes");
    assert_eq!(items.len(), 1);
    let change = &items[0];
    assert_eq!(change.path, "src/a.ts");
    assert_eq!(change.turn_index, 3);
    assert_eq!(change.additions, 10);
    assert_eq!(change.deletions, 2);
    assert_eq!(change.diff_text.as_deref(), Some("--- a\n"));
    assert!(change.reviewed);
    assert_eq!(change.updated_at, 1700000000000);
}

#[test]
fn decodes_golden_diff_fixture() {
    let msg: SessionFileChangeDiff =
        serde_json::from_str(fixture("change_diff.json").trim()).expect("decode diff");
    assert_eq!(msg.diff_text, "--- a\n");
}

#[test]
fn ignores_unknown_fields() {
    let items: Vec<SessionFileChange> = serde_json::from_str(
        r#"[{"path":"a","turnIndex":1,"status":"modified","additions":1,"deletions":0,"reviewed":false,"updatedAt":"2","futureField":1}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(items[0].path, "a");
}

#[test]
fn serializes_counts_as_numbers() {
    // uint32 counts stay JSON numbers (unlike int64 timestamps elsewhere).
    let raw = serde_json::to_string(&SessionFileChange {
        path: "src/a.ts".into(),
        turn_index: 3,
        status: "modified".into(),
        additions: 10,
        ..Default::default()
    })
    .expect("encode change");
    assert!(raw.contains(r#""turnIndex":3"#), "unexpected encoding: {raw}");
    assert!(!raw.contains("updatedAt"), "zero int64 must omit: {raw}");
}
