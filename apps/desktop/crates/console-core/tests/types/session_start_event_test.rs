use console_core::types::AgentSessionEvent;

#[test]
fn session_start_event_decodes_server_elapsed() {
    let json = r#"{
        "type": "sessionStart",
        "startedAt": "2026-10-13T09:45:44.936584122Z",
        "elapsedMs": 4200
    }"#;

    match serde_json::from_str::<AgentSessionEvent>(json).unwrap() {
        AgentSessionEvent::SessionStart {
            started_at,
            elapsed_ms,
        } => {
            assert_eq!(started_at.as_deref(), Some("2026-10-13T09:45:44.936584122Z"));
            assert_eq!(elapsed_ms, Some(4200));
        }
        other => panic!("wrong event: {other:?}"),
    }
}

#[test]
fn session_start_event_without_payload_still_decodes() {
    match serde_json::from_str::<AgentSessionEvent>(r#"{"type":"sessionStart"}"#).unwrap() {
        AgentSessionEvent::SessionStart {
            started_at,
            elapsed_ms,
        } => {
            assert_eq!(started_at, None);
            assert_eq!(elapsed_ms, None);
        }
        other => panic!("wrong event: {other:?}"),
    }
}
