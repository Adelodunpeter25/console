use console_proto::NotificationEvent;
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/notification")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_attention_fixture() {
    let n: NotificationEvent =
        serde_json::from_str(fixture("attention.json").trim()).expect("decode attention");
    assert_eq!(n.r#type, "notification");
    assert_eq!(n.kind, "needs_attention");
    assert_eq!(n.session_id, "s1");
    assert_eq!(n.title, "Needs Attention");
    assert_eq!(n.subtitle, "Fix the parser");
    assert_eq!(n.body, "sh is requesting permission");
}

#[test]
fn decodes_golden_done_fixture() {
    let n: NotificationEvent =
        serde_json::from_str(fixture("done.json").trim()).expect("decode done");
    assert_eq!(n.kind, "done");
    assert_eq!(n.title, "Done");
    assert_eq!(n.subtitle, "Fix the parser");
    assert_eq!(n.body, "all done here");
}

#[test]
fn missing_subtitle_defaults_to_blank() {
    // Absent key (old omitempty) must read as an empty subtitle, which the
    // state layer treats as a single-line banner.
    let n: NotificationEvent = serde_json::from_str(
        r#"{"type":"notification","kind":"done","sessionId":"s","title":"Done","body":"b"}"#,
    )
    .expect("decode without subtitle");
    assert_eq!(n.subtitle, "");
}

#[test]
fn ignores_unknown_fields() {
    let n: NotificationEvent = serde_json::from_str(
        r#"{"type":"notification","kind":"done","sessionId":"s","title":"t","body":"b","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(n.session_id, "s");
}