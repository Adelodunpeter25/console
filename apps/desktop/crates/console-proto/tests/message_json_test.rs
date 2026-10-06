use console_proto::{
    AgentMessage, agent_message::Message as MessageEvent,
    assistant_content_part::Part as PartEvent,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/message")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_user_fixture() {
    let msg: AgentMessage =
        serde_json::from_str(fixture("user.json").trim()).expect("decode user");
    match msg.message {
        Some(MessageEvent::User(u)) => {
            assert_eq!(u.content, "Hello");
            assert_eq!(u.context_files, vec!["/a.ts".to_string()]);
            assert_eq!(u.attachments.len(), 1);
        }
        _ => panic!("wrong variant"),
    }
    assert_eq!(msg.id, None);
}

#[test]
fn decodes_golden_assistant_fixture() {
    let msg: AgentMessage =
        serde_json::from_str(fixture("assistant.json").trim()).expect("decode assistant");
    match msg.message {
        Some(MessageEvent::Assistant(a)) => {
            assert_eq!(a.id, "m1");
            assert_eq!(a.content.len(), 3);
            match &a.content[2].part {
                Some(PartEvent::ToolCall(c)) => {
                    assert_eq!(c.id, "c1");
                    // Arguments cross as raw JSON bytes.
                    let args: serde_json::Value =
                        serde_json::from_slice(&c.arguments).expect("args json");
                    assert_eq!(args["path"], "/a");
                }
                _ => panic!("wrong part"),
            }
        }
        _ => panic!("wrong variant"),
    }
}

#[test]
fn decodes_golden_toolresult_fixture() {
    let msg: AgentMessage =
        serde_json::from_str(fixture("toolresult.json").trim()).expect("decode toolresult");
    match msg.message {
        Some(MessageEvent::ToolResult(t)) => {
            assert_eq!(t.results.len(), 1);
            assert_eq!(t.results[0].tool_call_id, "c1");
            let content: serde_json::Value =
                serde_json::from_slice(&t.results[0].content).expect("content json");
            assert_eq!(content["ok"], true);
        }
        _ => panic!("wrong variant"),
    }
}

#[test]
fn ignores_unknown_fields() {
    let msg: AgentMessage = serde_json::from_str(
        r#"{"user":{"content":"x","annotations":[{"id":"a"}]},"futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert!(matches!(msg.message, Some(MessageEvent::User(_))));
}
