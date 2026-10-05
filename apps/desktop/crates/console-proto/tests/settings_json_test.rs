use console_proto::ConsoleSettings;
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/settings")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_full_fixture() {
    let msg: ConsoleSettings =
        serde_json::from_str(fixture("full.json").trim()).expect("decode full fixture");
    let roles = msg.model_roles.expect("modelRoles present");
    assert_eq!(roles.vision.as_deref(), Some("openai/gpt-5"));
    assert_eq!(roles.smol.as_deref(), Some("anthropic/claude-opus-4"));
}

#[test]
fn decodes_golden_empty_fixture() {
    let msg: ConsoleSettings =
        serde_json::from_str(fixture("empty.json").trim()).expect("decode empty fixture");
    let roles = msg.model_roles.expect("modelRoles present even when empty");
    assert_eq!(roles.vision, None);
    assert_eq!(roles.smol, None);
}

#[test]
fn ignores_unknown_fields() {
    let msg: ConsoleSettings = serde_json::from_str(
        r#"{"modelRoles":{"vision":"x","futureRole":"y"},"futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(
        msg.model_roles.and_then(|r| r.vision).as_deref(),
        Some("x")
    );
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&ConsoleSettings {
        model_roles: Some(console_proto::ModelRoleMapping {
            vision: Some("openai/gpt-5".into()),
            smol: None,
        }),
    })
    .expect("encode settings");
    assert!(raw.contains("modelRoles"), "unexpected encoding: {raw}");
    assert!(raw.contains("vision"), "unexpected encoding: {raw}");
}
