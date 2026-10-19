//! Wire-to-render conversion coverage: canonical proto bytes decode into
//! the transcript render models through AgentMessage::from_proto.
use console_core::AgentMessage;
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

fn decode_fixture(name: &str) -> AgentMessage {
    let wire: console_proto::AgentMessage =
        serde_json::from_str(fixture(name).trim()).expect("decode fixture");
    AgentMessage::from_proto(wire).expect("convert fixture")
}

#[test]
fn converts_user_message() {
    match decode_fixture("user.json") {
        AgentMessage::User {
            content,
            attachments,
            context_files,
            ..
        } => {
            assert_eq!(content, "Hello");
            assert_eq!(attachments.unwrap().len(), 1);
            assert_eq!(context_files.unwrap(), vec!["/a.ts".to_string()]);
        }
        other => panic!("wrong variant: {other:?}"),
    }
}

#[test]
fn converts_assistant_message() {
    match decode_fixture("assistant.json") {
        AgentMessage::Assistant { content, .. } => {
            assert_eq!(content.len(), 3);
        }
        other => panic!("wrong variant: {other:?}"),
    }
}

#[test]
fn converts_tool_result_message() {
    match decode_fixture("toolresult.json") {
        AgentMessage::ToolResult { results, .. } => {
            assert_eq!(results.len(), 1);
            assert_eq!(results[0].tool_call_id, "c1");
            assert_eq!(results[0].content["ok"], true);
        }
        other => panic!("wrong variant: {other:?}"),
    }
}
