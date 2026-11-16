use console_core::types::SessionBootstrapResponse;

const HEADER: &str = r#"{"id":"s1","title":"Demo","cwd":"/tmp","projectId":"p1","modelId":"gpt-5","provider":"openai","approvalMode":"always-ask","thinkingLevel":"medium","createdAt":"1700000000000","updatedAt":"1700000000001","messageCount":3,"status":"done","worktree":{"path":"/tmp/wt","branch":"feat","repo":"/tmp"}}"#;

fn payload(messages: &str) -> String {
    format!(
        r#"{{"header":{HEADER},"messages":{messages},"hasMore":false,"nextCursor":null,"changes":[],"todos":[],"suggestions":[],"queuedPrompts":[],"backgroundWork":[],"draft":null}}"#
    )
}

#[test]
fn decodes_bootstrap_fields_through_flatten() {
    let value: SessionBootstrapResponse =
        serde_json::from_str(&payload("[]")).expect("bootstrap payload");
    assert_eq!(value.detail.header.id, "s1");
    assert!(value.detail.messages.is_empty());
    assert!(value.changes.is_empty());
    assert!(value.todos.is_empty());
    assert!(value.suggestions.is_empty());
    assert!(value.queued_prompts.is_empty());
    assert!(value.background_work.is_empty());
    assert!(value.draft.is_none());
}

#[test]
fn flatten_keeps_the_custom_message_decoder() {
    let messages = r#"[{"user":{"content":"Hello","contextFiles":["/a.ts"],"attachments":[{"data":"QUJD","mimeType":"image/png"}]}}]"#;
    let value: SessionBootstrapResponse =
        serde_json::from_str(&payload(messages)).expect("bootstrap payload with a user message");
    assert_eq!(value.detail.messages.len(), 1);
}
