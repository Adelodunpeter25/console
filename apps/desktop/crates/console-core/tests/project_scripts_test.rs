//! Serialization tests for the project run-script API types. Shapes must
//! match the server (`apps/server/api/src/services/project-scripts/`).

use console_core::{
    ProjectScript, ProjectScriptsResult, ScriptOutputStream, ScriptRun, ScriptRunEvent,
    ScriptRunStatus,
};

#[test]
fn test_project_script_defaults_match_server_normalization() {
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

    let minimal = r#"{"id": "build", "label": "Build", "command": "bun run build",
        "shortcut": null, "persistent": false}"#;
    let script: ProjectScript = serde_json::from_str(minimal).expect("deserialize minimal");
    assert_eq!(script.shortcut, None);
    assert!(!script.persistent);
}

#[test]
fn test_scripts_result_missing_source() {
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
        "exitCode": null, "stdout": "ready\n", "stderr": ""
    }"#;
    let run: ScriptRun = serde_json::from_str(json).expect("deserialize run");
    assert_eq!(run.run_id, "run-1");
    assert_eq!(run.status, ScriptRunStatus::Running);
    assert!(!run.status.is_terminal());

    let done = ScriptRun {
        status: ScriptRunStatus::Succeeded,
        exit_code: Some(0),
        ended_at: Some("2026-09-11T12:01:00.000Z".into()),
        ..run
    };
    assert!(done.status.is_terminal());
    let encoded = serde_json::to_string(&done).expect("serialize run");
    assert!(encoded.contains("\"runId\":\"run-1\""));
    assert!(encoded.contains("\"exitCode\":0"));
}

#[test]
fn test_run_events_parse_by_type_tag() {
    let status: ScriptRunEvent =
        serde_json::from_str(r#"{"type": "status", "status": "running"}"#).expect("status");
    assert_eq!(
        status,
        ScriptRunEvent::Status {
            status: ScriptRunStatus::Running
        }
    );

    let output: ScriptRunEvent = serde_json::from_str(
        r#"{"type": "output", "stream": "stderr", "text": "warning\n"}"#,
    )
    .expect("output");
    assert_eq!(
        output,
        ScriptRunEvent::Output {
            stream: ScriptOutputStream::Stderr,
            text: "warning\n".into()
        }
    );

    let exit: ScriptRunEvent =
        serde_json::from_str(r#"{"type": "exit", "status": "failed", "exitCode": 1}"#)
            .expect("exit");
    assert_eq!(
        exit,
        ScriptRunEvent::Exit {
            status: ScriptRunStatus::Failed,
            exit_code: Some(1)
        }
    );
}
