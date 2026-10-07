use console_proto::{
    AskQuestionRequest, BrowserActionRequest, ErrorPayload, PermissionRequest,
};
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

fn request_of<T: serde::de::DeserializeOwned>(name: &str) -> T {
    let raw = fixture(name);
    let value: serde_json::Value = serde_json::from_str(raw.trim()).expect("frame json");
    serde_json::from_value(value["request"].clone()).expect("request payload")
}

#[test]
fn decodes_golden_permission_frame() {
    let req: PermissionRequest = request_of("permission_request.json");
    assert_eq!(req.request_id, "r1");
    assert_eq!(req.tool_call_id, "c1");
    assert_eq!(req.tool_name, "sh");
    // Args cross as raw JSON bytes (base64 on the wire).
    assert_eq!(req.args, br#"{"path":"a"}"#);
    assert_eq!(req.tier, "write");
    assert_eq!(req.reason.as_deref(), Some("needs review"));
}

#[test]
fn decodes_golden_ask_frame() {
    let req: AskQuestionRequest = request_of("ask_question.json");
    assert_eq!(req.request_id, "r1");
    assert_eq!(req.question, "q?");
    assert_eq!(req.options, vec!["a".to_string(), "b".to_string()]);
    assert!(req.is_multi_select);
    // Explicit false stays present — never confused with absent.
    assert_eq!(req.skippable, Some(false));
    assert_eq!(req.batch_id.as_deref(), Some("b1"));
}

#[test]
fn decodes_golden_browser_frame() {
    let req: BrowserActionRequest = request_of("browser_action.json");
    assert_eq!(req.request_id, "r1");
    assert_eq!(req.action, "click");
    assert_eq!(req.selector.as_deref(), Some("#b"));
    assert_eq!(req.timeout_ms, 5000);
    assert_eq!(req.r#ref, None);
    assert!(!req.submit);
}

fn error_of(name: &str) -> ErrorPayload {
    let raw = fixture(name);
    let value: serde_json::Value = serde_json::from_str(raw.trim()).expect("frame json");
    serde_json::from_value(value["error"].clone()).expect("error payload")
}

#[test]
fn decodes_golden_error_frame() {
    assert_eq!(error_of("error.json").message, "boom");
}

#[test]
fn ignores_unknown_fields() {
    let req: PermissionRequest = serde_json::from_str(
        r#"{"requestId":"r","toolCallId":"c","toolName":"n","tier":"read","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(req.request_id, "r");
}
