use console_proto::QueuedPrompt;
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/session")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_queue_fixture() {
    let msg: QueuedPrompt =
        serde_json::from_str(fixture("queue.json").trim()).expect("decode queue");
    assert_eq!(msg.id, "q1");
    assert_eq!(msg.session_id, "s1");
    assert_eq!(msg.prompt, "fix it");
    assert_eq!(msg.context_files, vec!["a.ts".to_string()]);
    assert_eq!(msg.attachments.len(), 1);
    assert_eq!(msg.attachments[0].data, "aGk=");
    assert_eq!(msg.attachments[0].mime_type, "image/png");
    // Annotations ride the message schema; the desktop stores but never reads them.
    assert_eq!(msg.annotations.len(), 1);
    assert_eq!(msg.annotations[0].id, "an1");
    assert_eq!(msg.annotations[0].user_comment, "look");
    assert_eq!(msg.model_id.as_deref(), Some("m"));
    assert_eq!(msg.provider.as_deref(), Some("p"));
    assert_eq!(msg.approval_mode.as_deref(), Some("auto"));
    // Server never populates the desktop-optimistic thinking level.
    assert_eq!(msg.thinking_level, None);
    // created_at is the server's RFC3339 string, not millis.
    assert_eq!(msg.created_at, "2024-05-01T12:00:00Z");
}

#[test]
fn ignores_unknown_fields() {
    let msg: QueuedPrompt = serde_json::from_str(
        r#"{"id":"q","sessionId":"s","prompt":"hi","createdAt":"2024-05-01T12:00:00Z","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(msg.id, "q");
}

#[test]
fn omits_absent_optionals() {
    // Optional model/provider/approval stay absent (never empty strings).
    let raw = serde_json::to_string(&QueuedPrompt {
        id: "q".into(),
        session_id: "s".into(),
        prompt: "hi".into(),
        created_at: "2024-05-01T12:00:00Z".into(),
        ..Default::default()
    })
    .expect("encode queue");
    assert_eq!(
        raw,
        r#"{"id":"q","sessionId":"s","prompt":"hi","createdAt":"2024-05-01T12:00:00Z"}"#,
        "unexpected encoding: {raw}"
    );
}
