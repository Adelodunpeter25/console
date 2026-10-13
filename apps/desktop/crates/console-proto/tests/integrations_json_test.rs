use console_proto::{IntegrationStatus, IntegrationsResponse};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/integrations")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_response_fixture() {
    let msg: IntegrationsResponse =
        serde_json::from_str(fixture("response.json").trim()).expect("decode response");
    assert_eq!(msg.integrations.len(), 1);
    let gh = &msg.integrations[0];
    assert_eq!(gh.id, "gh");
    assert_eq!(gh.name, "GitHub CLI");
    // `installed` has explicit presence (optional bool), so it is Some(true)
    // rather than a bare bool.
    assert_eq!(gh.installed, Some(true));
    assert_eq!(gh.version.as_deref(), Some("2.75.0"));
    assert_eq!(gh.binary_path.as_deref(), Some("/opt/homebrew/bin/gh"));
    assert!(!gh.install_url.is_empty());
    assert!(!gh.install_hint.is_empty());
}

#[test]
fn missing_binary_carries_no_version_or_path() {
    // protojson drops unset optionals — a missing binary omits the keys, and
    // the install pointers are what the UI falls back to.
    let row: IntegrationStatus = serde_json::from_str(
        r#"{"id":"gh","name":"GitHub CLI","description":"d","installed":false,"installUrl":"https://cli.github.com","installHint":"sudo apt install gh"}"#,
    )
    .expect("decode missing row");
    assert_eq!(row.installed, Some(false));
    assert!(row.version.is_none());
    assert!(row.binary_path.is_none());
}
