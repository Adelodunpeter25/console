//! Shell helpers on background work: the running-shell filter, the command
//! label, and the output status line.
use console_core::types::{AgentSessionEvent, BackgroundWorkOutput};

fn work(kind: &str, status: &str, meta: &str, summary: &str) -> console_core::types::BackgroundWork {
    let json = format!(
        r#"{{"type":"backgroundWorkStarted","work":{{
            "id":"work_abcdef123","sessionId":"s1","kind":"{kind}","status":"{status}",
            "createdAt":"2026-10-13T09:45:44Z","summary":"{summary}","metadata":{meta}}}}}"#
    );
    match serde_json::from_str::<AgentSessionEvent>(&json).unwrap() {
        AgentSessionEvent::BackgroundWorkStarted { work } => work,
        other => panic!("wrong event: {other:?}"),
    }
}

#[test]
fn only_running_shells_count() {
    assert!(work("shell", "running", "{}", "").is_running_shell());
    assert!(!work("shell", "completed", "{}", "").is_running_shell());
    assert!(!work("subagent", "running", "{}", "").is_running_shell());
}

#[test]
fn shell_command_prefers_command_then_summary_then_id() {
    let with_cmd = work("shell", "running", r#"{"command":"cargo check\nsecond"}"#, "sum");
    assert_eq!(with_cmd.shell_command(), "cargo check");
    let with_summary = work("shell", "running", "{}", "Running tests");
    assert_eq!(with_summary.shell_command(), "Running tests");
    let bare = work("shell", "running", "{}", "");
    assert_eq!(bare.shell_command(), "Shell work_a");
}

#[test]
fn output_decodes_and_labels_status() {
    let out: BackgroundWorkOutput = serde_json::from_str(
        r#"{"stdout":"hi","stderr":"warn","nextCursor":2,"truncated":false,
            "status":"exited","exitCode":0,"timedOut":false}"#,
    )
    .unwrap();
    assert_eq!(out.combined(), "hi\nwarn");
    assert_eq!(out.status_label(), "Exited · code 0");
    assert!(!out.is_running());

    let running: BackgroundWorkOutput = serde_json::from_str(r#"{"status":"running"}"#).unwrap();
    assert!(running.is_running());
    assert_eq!(running.status_label(), "Running");

    let timed: BackgroundWorkOutput =
        serde_json::from_str(r#"{"status":"killed","timedOut":true}"#).unwrap();
    assert_eq!(timed.status_label(), "Timed out");
}
