use console_proto::{DeleteProjectResponse, ProjectInfo};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/projects")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_project_fixture() {
    // Timestamps arrive as protojson strings, not numbers.
    let msg: ProjectInfo =
        serde_json::from_str(fixture("project.json").trim()).expect("decode project fixture");
    assert_eq!(msg.id, "p1");
    assert_eq!(msg.name, "demo");
    assert_eq!(msg.path, "/tmp/demo");
    assert_eq!(msg.created_at, 1700000000000);
    assert_eq!(msg.updated_at, 1700000000001);
}

#[test]
fn decodes_golden_list_fixture() {
    let items: Vec<ProjectInfo> =
        serde_json::from_str(fixture("list.json").trim()).expect("decode list fixture");
    assert_eq!(items.len(), 1);
    assert_eq!(items[0].id, "p1");
}

#[test]
fn decodes_golden_delete_fixture() {
    let msg: DeleteProjectResponse =
        serde_json::from_str(fixture("delete.json").trim()).expect("decode delete fixture");
    assert_eq!(msg.id, "p1");
    assert!(msg.deleted);
}

#[test]
fn ignores_unknown_fields() {
    let msg: ProjectInfo = serde_json::from_str(
        r#"{"id":"a","name":"b","path":"/c","createdAt":"1","updatedAt":"2","futureField":"x"}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(msg.id, "a");
}

#[test]
fn serializes_timestamps_as_strings() {
    let raw = serde_json::to_string(&ProjectInfo {
        id: "p1".into(),
        name: "demo".into(),
        path: "/tmp/demo".into(),
        created_at: 1700000000000,
        updated_at: 1700000000001,
    })
    .expect("encode project");
    assert!(
        raw.contains(r#""createdAt":"1700000000000""#),
        "unexpected encoding: {raw}"
    );
}
