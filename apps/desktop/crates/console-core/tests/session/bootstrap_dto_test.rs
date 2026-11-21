use console_core::types::{SessionBootstrapResponse, SessionDetailResponse};
use prost::Message;

const HEADER: &str = r#"{"id":"s1","title":"Demo","cwd":"/tmp","projectId":"p1","modelId":"gpt-5","provider":"openai","approvalMode":"always-ask","thinkingLevel":"medium","createdAt":"1700000000000","updatedAt":"1700000000001","messageCount":3,"status":"done","worktree":{"path":"/tmp/wt","branch":"feat","repo":"/tmp"}}"#;

fn payload(turns: &str) -> String {
    format!(
        r#"{{"header":{HEADER},"turns":{turns},"hasMore":false,"nextCursor":null,"changes":[],"todos":[],"suggestions":[],"queuedPrompts":[],"backgroundWork":[],"draft":null}}"#
    )
}

#[test]
fn decodes_bootstrap_fields_through_flatten() {
    let value: SessionBootstrapResponse =
        serde_json::from_str(&payload("[]")).expect("bootstrap payload");
    assert_eq!(value.detail.header.id, "s1");
    assert!(value.detail.turns.is_empty());
    assert!(value.changes.is_empty());
    assert!(value.todos.is_empty());
    assert!(value.suggestions.is_empty());
    assert!(value.queued_prompts.is_empty());
    assert!(value.background_work.is_empty());
    assert!(value.draft.is_none());
}

#[test]
fn flatten_keeps_the_custom_message_decoder() {
    let turns = r#"[{"id":"t1","createdAt":"1700000000000","userMessage":{"user":{"content":"Hello","contextFiles":["/a.ts"],"attachments":[{"data":"QUJD","mimeType":"image/png"}]}}}]"#;
    let value: SessionBootstrapResponse =
        serde_json::from_str(&payload(turns)).expect("bootstrap payload with a user message");
    assert_eq!(value.detail.turns.len(), 1);
}

#[test]
fn decodes_binary_bootstrap_envelope() {
    let wire = console_proto::SessionBootstrapResponse {
        header: Some(console_proto::SessionHeader {
            id: "s1".into(),
            ..Default::default()
        }),
        turns: vec![console_proto::TranscriptTurn {
            id: "t1".into(),
            created_at: 1700000000000,
            user_message: Some(console_proto::AgentMessage {
                message: Some(console_proto::agent_message::Message::User(
                    console_proto::AgentUserMessage {
                        content: "Hello".into(),
                        ..Default::default()
                    },
                )),
                ..Default::default()
            }),
            ..Default::default()
        }],
        todos: vec![console_proto::TodoItem {
            id: 1,
            content: "task".into(),
            status: "pending".into(),
        }],
        background_work: vec![console_proto::BackgroundWork {
            id: "w1".into(),
            kind: "shell".into(),
            status: "running".into(),
            ..Default::default()
        }],
        ..Default::default()
    };
    let decoded = console_proto::SessionBootstrapResponse::decode(wire.encode_to_vec().as_slice())
        .expect("encode/decode round trip");
    let value = SessionBootstrapResponse::from_proto(decoded);
    assert_eq!(value.detail.header.id, "s1");
    assert_eq!(value.detail.turns.len(), 1);
    assert_eq!(value.todos[0].content, "task");
    assert_eq!(value.background_work[0].id, "w1");
}

#[test]
fn decodes_binary_history_page() {
    let wire = console_proto::SessionMessagesPage {
        header: Some(console_proto::SessionHeader {
            id: "s1".into(),
            ..Default::default()
        }),
        turns: vec![console_proto::TranscriptTurn {
            id: "t1".into(),
            created_at: 1700000000000,
            user_message: Some(console_proto::AgentMessage {
                message: Some(console_proto::agent_message::Message::User(
                    console_proto::AgentUserMessage {
                        content: "Hello".into(),
                        ..Default::default()
                    },
                )),
                ..Default::default()
            }),
            ..Default::default()
        }],
        has_more: true,
        next_cursor: Some(42),
    };
    let decoded = console_proto::SessionMessagesPage::decode(wire.encode_to_vec().as_slice())
        .expect("encode/decode round trip");
    let page = SessionDetailResponse::from_proto_page(decoded);
    assert_eq!(page.header.id, "s1");
    assert_eq!(page.turns.len(), 1);
    assert!(page.has_more);
    assert_eq!(page.next_cursor, Some(42));
}
