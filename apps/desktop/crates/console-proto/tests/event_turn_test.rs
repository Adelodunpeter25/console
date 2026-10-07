use console_proto::AgentAssistantMessage;
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
struct TurnFrame {
    #[serde(rename = "type")]
    kind: String,
    #[serde(rename = "turnId")]
    turn_id: String,
    turn: AgentAssistantMessage,
}

#[test]
fn decodes_golden_turn_frame() {
    let frame: TurnFrame =
        serde_json::from_str(fixture("model_stream_end.json").trim()).expect("decode turn");
    assert_eq!(frame.kind, "modelStreamEnd");
    assert_eq!(frame.turn_id, "t1");
    assert_eq!(frame.turn.id, "a1");
    assert_eq!(frame.turn.stop_reason, "end_turn");
    assert_eq!(frame.turn.content.len(), 2);
    // The render model recovers text and tool args (mirrors
    // AssistantMessage::from_proto_assistant).
    let render_content: Vec<serde_json::Value> = frame
        .turn
        .content
        .iter()
        .map(|p| {
            serde_json::to_value(p).expect("part must serialize")
        })
        .collect();
    assert!(render_content[0].get("text").is_some());
    assert_eq!(
        render_content[1].get("toolCall").and_then(|t| t.get("id")),
        Some(&serde_json::json!("c1")),
    );
}

// Hand ContextSnapshot shape (wire-identical numbers; the schema owns the
// shape, this type decodes unchanged). console-proto cannot depend on
// console-core, so the shape is restated here in full.
#[derive(serde::Deserialize, PartialEq, Debug)]
#[serde(rename_all = "camelCase")]
struct SnapshotShape {
    #[serde(default)]
    used_tokens: i64,
    #[serde(default)]
    context_window: i64,
    #[serde(default)]
    percent_used: f64,
    #[serde(default)]
    threshold_ratio: f64,
    #[serde(default)]
    model_id: String,
    #[serde(default)]
    provider: String,
    #[serde(default)]
    source: String,
}

#[test]
fn decodes_golden_context_frame() {
    #[derive(serde::Deserialize)]
    struct Raw {
        #[allow(dead_code)]
        #[serde(rename = "type")]
        kind: String,
        context: SnapshotShape,
    }
    let frame: Raw =
        serde_json::from_str(fixture("context_update.json").trim()).expect("decode context");
    assert_eq!(
        frame.context,
        SnapshotShape {
            used_tokens: 25000,
            context_window: 200000,
            percent_used: 12.5,
            threshold_ratio: 0.8,
            model_id: "m".into(),
            provider: "p".into(),
            source: "live".into(),
        }
    );
}

#[test]
fn ignores_unknown_fields() {
    let frame: TurnFrame = serde_json::from_str(
        r#"{"type":"modelStreamEnd","turnId":"t","turn":{"id":"a","content":[],"stopReason":"","futureField":1}}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(frame.turn.id, "a");
}
