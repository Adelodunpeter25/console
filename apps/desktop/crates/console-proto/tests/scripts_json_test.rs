use console_proto::{
    ScriptRun, ScriptRunEvent, script_run_event::Event as ScriptRunEventKind,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/scripts")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_script_fixture() {
    let msg: console_proto::ProjectScript =
        serde_json::from_str(fixture("script.json").trim()).expect("decode script fixture");
    assert_eq!(msg.id, "dev");
    assert_eq!(msg.command, "bun dev");
    // Unset shortcuts encode as "" (never null, never omitted).
    assert_eq!(msg.shortcut.as_deref(), Some(""));
    assert!(msg.persistent);
}

#[test]
fn decodes_golden_result_fixture() {
    let msg: console_proto::ProjectScriptsResult =
        serde_json::from_str(fixture("scripts_result.json").trim())
            .expect("decode result fixture");
    assert_eq!(msg.project_id, "p1");
    assert_eq!(msg.scripts.len(), 1);
    assert_eq!(msg.source, "console.toml");
}

#[test]
fn decodes_golden_run_fixture() {
    let msg: ScriptRun =
        serde_json::from_str(fixture("run.json").trim()).expect("decode run fixture");
    assert_eq!(msg.run_id, "r1");
    assert_eq!(msg.status, "running");
    assert_eq!(msg.stdout, "hi\n");
    assert_eq!(msg.exit_code, None);
}

#[test]
fn decodes_golden_event_fixtures() {
    let status: ScriptRunEvent =
        serde_json::from_str(fixture("event_status.json").trim()).expect("status");
    assert!(matches!(
        status.event,
        Some(ScriptRunEventKind::Status(_))
    ));
    let output: ScriptRunEvent =
        serde_json::from_str(fixture("event_output.json").trim()).expect("output");
    match output.event {
        Some(ScriptRunEventKind::Output(e)) => assert_eq!(e.stream, "stdout"),
        _ => panic!("wrong variant"),
    }
    let exit: ScriptRunEvent =
        serde_json::from_str(fixture("event_exit.json").trim()).expect("exit");
    match exit.event {
        Some(ScriptRunEventKind::Exit(e)) => assert_eq!(e.exit_code, Some(3)),
        _ => panic!("wrong variant"),
    }
}

#[test]
fn ignores_unknown_fields() {
    let msg: ScriptRun = serde_json::from_str(
        r#"{"runId":"r","projectId":"p","scriptId":"s","label":"l","status":"running","startedAt":"t","stdout":"","stderr":"","signal":"SIGKILL"}"#,
    )
    .expect("dropped signal field must be ignored");
    assert_eq!(msg.run_id, "r");
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&ScriptRun {
        run_id: "r1".into(),
        status: "running".into(),
        ..Default::default()
    })
    .expect("encode run");
    assert!(raw.contains(r#""runId":"r1""#), "unexpected encoding: {raw}");
}
