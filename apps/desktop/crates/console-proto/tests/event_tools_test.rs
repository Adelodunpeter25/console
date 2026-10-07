use console_proto::{ToolCall, ToolResult};
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
struct StartFrame {
    #[serde(rename = "type")]
    kind: String,
    calls: Vec<ToolCall>,
}

#[derive(serde::Deserialize)]
struct ResultFrame {
    #[allow(dead_code)]
    #[serde(rename = "type")]
    kind: String,
    result: ToolResult,
}

#[derive(serde::Deserialize)]
struct EndFrame {
    #[allow(dead_code)]
    #[serde(rename = "type")]
    kind: String,
    results: Vec<ToolResult>,
}

#[test]
fn decodes_golden_start_frame() {
    let frame: StartFrame =
        serde_json::from_str(fixture("tool_execution_start.json").trim()).expect("decode start");
    assert_eq!(frame.kind, "toolExecutionStart");
    assert_eq!(frame.calls.len(), 2);
    // Arguments cross as raw JSON bytes (base64 on the wire).
    assert_eq!(frame.calls[0].arguments, br#"{"cmd":"ls"}"#);
    assert_eq!(frame.calls[0].thought_signature.as_deref(), Some("sig"));
    assert!(frame.calls[1].arguments.is_empty());
    assert_eq!(frame.calls[1].thought_signature, None);
}

#[test]
fn decodes_golden_result_frame() {
    let frame: ResultFrame =
        serde_json::from_str(fixture("tool_execution_result.json").trim()).expect("decode result");
    assert_eq!(frame.result.tool_call_id, "c1");
    assert_eq!(frame.result.tool_name.as_deref(), Some("sh"));
    assert_eq!(frame.result.content, br#""ok""#);
    assert_eq!(frame.result.is_error, None);
    // The render model recovers the JSON value from the bytes (mirrors
    // ToolResult::from_proto; console-proto cannot depend on console-core).
    let content: serde_json::Value =
        serde_json::from_slice(&frame.result.content).unwrap_or(serde_json::Value::Null);
    assert_eq!(content, serde_json::json!("ok"));
}

#[test]
fn decodes_golden_end_frame() {
    let frame: EndFrame =
        serde_json::from_str(fixture("tool_execution_end.json").trim()).expect("decode end");
    assert_eq!(frame.results.len(), 2);
    assert_eq!(frame.results[0].content, br#"{"a":2,"b":1}"#);
    assert_eq!(frame.results[1].is_error, Some(true));
}

#[test]
fn ignores_unknown_fields() {
    let frame: ResultFrame = serde_json::from_str(
        r#"{"type":"toolExecutionResult","result":{"toolCallId":"c","content":"bnVsbA==","futureField":1}}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(frame.result.tool_call_id, "c");
}
