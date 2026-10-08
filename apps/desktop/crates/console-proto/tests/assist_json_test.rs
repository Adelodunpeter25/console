use console_proto::{AssistFileSearchResponse, SlashCommandInfo};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/assist")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_commands_fixture() {
    let cmds: Vec<SlashCommandInfo> =
        serde_json::from_str(fixture("commands.json").trim()).expect("decode commands fixture");
    assert_eq!(cmds.len(), 3);
    assert_eq!(cmds[0].name, "init");
    assert!(cmds[0].builtin);
    assert_eq!(cmds[1].name, "computer-use");
    assert!(cmds[1].builtin);
    // A discovered skill omits `builtin` on the wire and decodes to false —
    // the protojson zero, not a missing concept.
    assert_eq!(cmds[2].name, "find-skills");
    assert!(!cmds[2].builtin);
}

#[test]
fn decodes_golden_search_fixture() {
    let msg: AssistFileSearchResponse =
        serde_json::from_str(fixture("search.json").trim()).expect("decode search fixture");
    assert_eq!(msg.root, "/p");
    assert_eq!(msg.query, "DiffView");
    assert_eq!(msg.items.len(), 2);
    assert_eq!(msg.items[0].relative_path, "src/DiffView.kt");
    assert_eq!(msg.items[0].absolute_path, "/p/src/DiffView.kt");
    assert!(!msg.items[0].is_dir);
    assert!((msg.items[0].score - 0.95).abs() < f64::EPSILON);
    assert!(msg.items[1].is_dir);
}

#[test]
fn decodes_empty_query_fixture() {
    // The wire omits `query` for a bare "@"; the client still reads "".
    let msg: AssistFileSearchResponse = serde_json::from_str(fixture("search_empty_query.json").trim())
        .expect("decode empty-query fixture");
    assert_eq!(msg.query, "");
    assert_eq!(msg.items.len(), 1);
    assert_eq!(msg.items[0].relative_path, "AGENTS.md");
    assert_eq!(msg.items[0].score, 0.0);
}

#[test]
fn ignores_unknown_fields() {
    let cmds: Vec<SlashCommandInfo> = serde_json::from_str(
        r#"[{"name":"a","description":"b","builtin":true,"futureField":"x"}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(cmds[0].name, "a");
    assert!(cmds[0].builtin);
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&SlashCommandInfo {
        name: "a".into(),
        description: "b".into(),
        builtin: true,
    })
    .expect("encode command");
    assert!(raw.contains(r#""name":"a""#), "unexpected encoding: {raw}");
    assert!(raw.contains(r#""builtin":true"#), "unexpected encoding: {raw}");
}