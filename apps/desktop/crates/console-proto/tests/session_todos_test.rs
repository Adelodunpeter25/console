use console_proto::TodoItem;
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
fn decodes_golden_todos_fixture() {
    let items: Vec<TodoItem> =
        serde_json::from_str(fixture("todos.json").trim()).expect("decode todos");
    assert_eq!(items.len(), 2);
    assert_eq!(items[0].id, 1);
    assert_eq!(items[0].content, "Write code");
    assert_eq!(items[0].status, "in_progress");
    assert_eq!(items[1].status, "pending");
}

#[test]
fn ignores_unknown_fields() {
    let items: Vec<TodoItem> = serde_json::from_str(
        r#"[{"id":1,"content":"x","status":"pending","futureField":1}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(items[0].id, 1);
}

#[test]
fn serializes_ids_as_numbers() {
    // int32 ids stay JSON numbers (unlike int64 timestamps elsewhere).
    let raw = serde_json::to_string(&TodoItem {
        id: 1,
        content: "Write code".into(),
        status: "in_progress".into(),
    })
    .expect("encode todo");
    assert!(raw.contains(r#""id":1"#), "unexpected encoding: {raw}");
}
