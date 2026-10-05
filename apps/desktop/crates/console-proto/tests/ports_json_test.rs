use console_proto::{ForwardedPort, UnforwardPortResponse};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/ports")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_port_fixture() {
    let msg: ForwardedPort =
        serde_json::from_str(fixture("port.json").trim()).expect("decode port fixture");
    assert_eq!(msg.port, 5173);
    assert_eq!(msg.url, "http://localhost:45173");
    assert_eq!(msg.project_id, None);
}

#[test]
fn decodes_golden_list_fixture() {
    let items: Vec<ForwardedPort> =
        serde_json::from_str(fixture("list.json").trim()).expect("decode list fixture");
    assert_eq!(items.len(), 2);
    assert_eq!(items[0].port, 3000);
    assert_eq!(items[1].project_id.as_deref(), Some("p1"));
}

#[test]
fn decodes_golden_unforward_fixture() {
    let msg: UnforwardPortResponse =
        serde_json::from_str(fixture("unforward.json").trim()).expect("decode unforward fixture");
    assert_eq!(msg.port, 5173);
}

#[test]
fn ignores_unknown_fields() {
    let msg: ForwardedPort = serde_json::from_str(
        r#"{"port":80,"url":"http://x","futureField":"y"}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(msg.port, 80);
}

#[test]
fn serializes_port_as_number() {
    // int32 stays a JSON number (unlike int64 timestamps elsewhere).
    let raw = serde_json::to_string(&ForwardedPort {
        port: 5173,
        url: "http://localhost:45173".into(),
        project_id: None,
    })
    .expect("encode port");
    assert!(raw.contains(r#""port":5173"#), "unexpected encoding: {raw}");
    assert!(!raw.contains(r#""projectId""#), "unset fields must omit: {raw}");
}
