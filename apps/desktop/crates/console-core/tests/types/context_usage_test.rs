use console_core::types::*;

#[test]
fn test_context_snapshot_deserialization() {
    let json_data = r#"{
        "usedTokens": 23100,
        "contextWindow": 1000000,
        "percentUsed": 2.31,
        "thresholdRatio": 0.85,
        "modelId": "gemini-2.5-pro",
        "provider": "antigravity",
        "source": "local"
    }"#;
    let snapshot: ContextSnapshot = serde_json::from_str(json_data).unwrap();
    assert_eq!(snapshot.used_tokens, 23100);
    assert_eq!(snapshot.context_window, 1000000);
    assert!((snapshot.used_fraction() - 0.0231).abs() < 1e-9);
    assert_eq!(snapshot.used_vs_window(), "23.1k/1.0M");
}

#[test]
fn test_context_update_event_deserialization() {
    let json_data = r#"{
        "type": "contextUpdate",
        "context": {
            "usedTokens": 50000,
            "contextWindow": 200000,
            "percentUsed": 25.0,
            "thresholdRatio": 0.85,
            "modelId": "claude-opus-4-6",
            "provider": "claude",
            "source": "local"
        }
    }"#;
    let event: AgentSessionEvent = serde_json::from_str(json_data).unwrap();
    match event {
        AgentSessionEvent::ContextUpdate { context } => {
            assert_eq!(context.used_tokens, 50000);
            assert_eq!(context.used_vs_window(), "50.0k/200.0k");
        }
        other => panic!("wrong variant: {other:?}"),
    }
}
