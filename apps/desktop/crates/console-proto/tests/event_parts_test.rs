use console_proto::ModelStreamPart;
use console_proto::model_stream_part::Part as PartKind;
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

#[derive(serde::Deserialize)]
struct Frame {
    #[serde(rename = "type")]
    kind: String,
    part: ModelStreamPart,
}

fn decode_frame(name: &str) -> (String, ModelStreamPart) {
    let frame: Frame = serde_json::from_str(fixture(name).trim()).expect("decode frame");
    (frame.kind, frame.part)
}

#[test]
fn decodes_golden_text_part() {
    let (kind, part) = decode_frame("stream_part_text.json");
    assert_eq!(kind, "modelStreamPart");
    assert!(matches!(part.part, Some(PartKind::Text(ref t)) if t == "hi"));
}

#[test]
fn decodes_golden_thinking_part() {
    let (kind, part) = decode_frame("stream_part_thinking.json");
    assert_eq!(kind, "modelStreamPart");
    assert!(matches!(part.part, Some(PartKind::Thinking(ref t)) if t == "hmm"));
}

#[test]
fn decodes_golden_tool_call_part() {
    let (_, part) = decode_frame("stream_part_tool_call.json");
    match part.part {
        Some(PartKind::ToolCall(preview)) => {
            assert_eq!(preview.id, "c1");
            assert_eq!(preview.name, "sh");
        }
        other => panic!("unexpected part: {other:?}"),
    }
}

#[test]
fn ignores_unknown_fields() {
    let frame: Frame = serde_json::from_str(
        r#"{"type":"modelStreamPart","part":{"text":"hi","futureField":1}}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(frame.kind, "modelStreamPart");
}

#[test]
fn serializes_flat_oneof() {
    // Scalars sit directly under part (no wrapper level).
    let raw = serde_json::to_string(&ModelStreamPart {
        part: Some(PartKind::Text("hi".into())),
        ..Default::default()
    })
    .expect("encode part");
    assert_eq!(raw, r#"{"text":"hi"}"#, "unexpected encoding: {raw}");
}
