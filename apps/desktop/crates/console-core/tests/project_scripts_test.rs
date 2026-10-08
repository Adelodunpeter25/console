//! Serialization tests for the project run-script API types. Shapes must
//! match the shared golden fixtures in proto/testdata/scripts (oneof event
//! frames, string statuses, "" for unset shortcuts).

use console_core::{
    ProjectScript, ProjectScriptsResult, ScriptRun, ScriptRunEvent, ScriptRunEventKind,
    script_run_is_terminal,
};

// Decodes a script definition, including the "" shortcut the wire uses for unset.
#[test]
fn test_project_script_decode() {
    let json = r#"{
        "id": "dev",
        "label": "Start Dev",
        "command": "bun run dev",
        "shortcut": "cmd-r",
        "persistent": true
    }"#;
    let script: ProjectScript = serde_json::from_str(json).expect("deserialize script");
    assert_eq!(script.shortcut.as_deref(), Some("cmd-r"));
    assert!(script.persistent);

    // The wire encodes unset shortcuts as "" (proto3 cannot emit null);
    // the service normalizes back to None, but the type decodes as Some("").
    let minimal = r#"{"id": "build", "label": "Build", "command": "bun run build",
        "shortcut": "", "persistent": false}"#;
    let script: ProjectScript = serde_json::from_str(minimal).expect("deserialize minimal");
    assert_eq!(script.shortcut.as_deref(), Some(""));
    assert!(!script.persistent);
}

// Decodes the scripts list response for a project, including the "missing" source.
#[test]
fn test_scripts_result_decode() {
    let json = r#"{"projectId": "proj-1", "scripts": [], "source": "missing"}"#;
    let result: ProjectScriptsResult = serde_json::from_str(json).expect("deserialize result");
    assert_eq!(result.project_id, "proj-1");
    assert!(result.scripts.is_empty());
    assert_eq!(result.source, "missing");
}

#[test]
fn test_run_record_camel_case_round_trip() {
    let json = r#"{
        "runId": "run-1", "projectId": "proj-1", "scriptId": "dev",
        "label": "Start Dev", "persistent": true, "status": "running",
        "startedAt": "2026-09-11T12:00:00.000Z", "endedAt": null,
        "exitCode": null, "stdout": "ready\n"
    }"#;
    let run: ScriptRun = serde_json::from_str(json).expect("deserialize run");
    assert_eq!(run.run_id, "run-1");
    assert_eq!(run.status, "running");
    assert!(!script_run_is_terminal(&run.status));

    let done = ScriptRun {
        status: "succeeded".to_string(),
        exit_code: Some(0),
        ended_at: Some("2026-09-11T12:01:00.000Z".into()),
        ..run
    };
    assert!(script_run_is_terminal(&done.status));
    let encoded = serde_json::to_string(&done).expect("serialize run");
    assert!(encoded.contains("\"runId\":\"run-1\""));
    assert!(encoded.contains("\"exitCode\":0"));
}

#[test]
fn test_run_events_parse_oneof_shape() {
    let status: ScriptRunEvent =
        serde_json::from_str(r#"{"status": {"status": "running"}}"#).expect("status");
    match status.event {
        Some(ScriptRunEventKind::Status(e)) => assert_eq!(e.status, "running"),
        _ => panic!("wrong variant"),
    }

    let output: ScriptRunEvent = serde_json::from_str(
        r#"{"output": {"stream": "stderr", "text": "warning\n"}}"#,
    )
    .expect("output");
    match output.event {
        Some(ScriptRunEventKind::Output(e)) => {
            assert_eq!(e.stream, "stderr");
            assert_eq!(e.text, "warning\n");
        }
        _ => panic!("wrong variant"),
    }

    let exit: ScriptRunEvent =
        serde_json::from_str(r#"{"exit": {"status": "failed", "exitCode": 1}}"#).expect("exit");
    match exit.event {
        Some(ScriptRunEventKind::Exit(e)) => {
            assert_eq!(e.status, "failed");
            assert_eq!(e.exit_code, Some(1));
        }
        _ => panic!("wrong variant"),
    }
}
