use console_proto::GetApiVersionResponse;
use std::path::PathBuf;

fn fixture_path() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../..")
        .join("proto/testdata/common/version.json")
}

#[test]
fn decodes_golden_version_fixture() {
    let raw = std::fs::read_to_string(fixture_path()).expect("read version.json fixture");
    let msg: GetApiVersionResponse = serde_json::from_str(&raw).expect("decode version fixture");
    assert_eq!(msg.api_version, 1);
}

#[test]
fn ignores_unknown_fields() {
    let msg: GetApiVersionResponse =
        serde_json::from_str(r#"{"apiVersion":1,"futureField":"x"}"#)
            .expect("unknown fields must be ignored");
    assert_eq!(msg.api_version, 1);
}

#[test]
fn serializes_camel_case() {
    let msg = GetApiVersionResponse { api_version: 1 };
    let raw = serde_json::to_string(&msg).expect("encode version");
    assert!(raw.contains("apiVersion"), "unexpected encoding: {raw}");
}
