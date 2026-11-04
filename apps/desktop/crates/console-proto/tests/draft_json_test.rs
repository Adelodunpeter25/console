use console_proto::PutDraftResponse;
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/drafts")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_put_response_fixture() {
    let response: PutDraftResponse =
        serde_json::from_str(fixture("put_response.json").trim()).expect("decode put response");
    assert!(response.applied);
    let draft = response.draft.expect("draft");
    assert_eq!(draft.session_id, "s1");
    assert_eq!(draft.text, "review @src/a.rs");
    assert_eq!(draft.updated_at, 1_700_000_000_000);
    assert_eq!(draft.mentions[0].path, "/repo/src/a.rs");
    assert_eq!(draft.comments[0].start_line, 3);
    assert_eq!(draft.comments[0].end_line, 5);
    assert_eq!(draft.comments[0].text, "rename this");
}

#[test]
fn encodes_to_the_wire_shape_the_server_reads() {
    let response: PutDraftResponse =
        serde_json::from_str(fixture("put_response.json").trim()).unwrap();
    let encoded = serde_json::to_value(&response).unwrap();
    let golden: serde_json::Value = serde_json::from_str(fixture("put_response.json").trim()).unwrap();
    assert_eq!(encoded, golden);
}
