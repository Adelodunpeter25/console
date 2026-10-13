use console_core::types::AgentSessionEvent;

#[test]
fn background_work_events_deserialize_without_nested_event_payload() {
    let json = r#"{
        "type": "backgroundWorkCompleted",
        "work": {
            "id": "work_test",
            "sessionId": "session-1",
            "parentRunId": "run_test",
            "parentCallId": "call_test",
            "kind": "shell",
            "status": "completed",
            "createdAt": "2026-10-13T09:45:44.936584122Z",
            "startedAt": "2026-10-13T09:45:44.936584122Z",
            "finishedAt": "2026-10-13T09:45:45.936584122Z",
            "summary": "Command finished with status exited",
            "metadata": {"job_id": "job_test", "exit_code": "0"}
        }
    }"#;

    let event: AgentSessionEvent = serde_json::from_str(json).unwrap();
    match event {
        AgentSessionEvent::BackgroundWorkCompleted { work } => {
            assert_eq!(work.id, "work_test");
            assert_eq!(work.kind, "shell");
            assert_eq!(work.status, "completed");
            assert_eq!(
                work.metadata.get("job_id").map(String::as_str),
                Some("job_test")
            );
        }
        other => panic!("wrong event: {other:?}"),
    }
}
