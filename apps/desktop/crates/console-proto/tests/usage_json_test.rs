use console_proto::{UsageLimit, UsageReport};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/usage")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_report_fixture() {
    let msg: UsageReport =
        serde_json::from_str(fixture("report.json").trim()).expect("decode report fixture");
    assert_eq!(msg.provider, "claude");
    // Timestamps arrive as protojson strings, not numbers.
    assert_eq!(msg.fetched_at, 1700000000000);
    assert_eq!(msg.limits.len(), 1);
    let limit = &msg.limits[0];
    assert_eq!(limit.id, "session");
    let scope = limit.scope.as_ref().expect("scope present");
    assert_eq!(scope.provider, "claude");
    assert_eq!(scope.tier.as_deref(), Some("pro"));
    let window = limit.window.as_ref().expect("window present");
    assert_eq!(window.resets_at, Some(1700003600000));
    let amount = limit.amount.as_ref().expect("amount present");
    assert_eq!(amount.used, Some(12.5));
    assert_eq!(amount.unit, "percent");
    assert_eq!(limit.status.as_deref(), Some("ok"));
}

#[test]
fn decodes_golden_map_fixture() {
    let map: std::collections::HashMap<String, Option<UsageReport>> =
        serde_json::from_str(fixture("map.json").trim()).expect("decode map fixture");
    assert_eq!(map.len(), 3);
    assert!(map["antigravity"].is_none());
    assert!(map["codex"].is_none());
    assert_eq!(map["claude"].as_ref().unwrap().provider, "claude");
}

#[test]
fn ignores_unknown_fields() {
    let msg: UsageReport = serde_json::from_str(
        r#"{"provider":"c","fetchedAt":"1","limits":[],"metadata":{"x":1},"raw":{},"resetCredits":{"availableCount":2}}"#,
    )
    .expect("dropped fields must be ignored");
    assert_eq!(msg.provider, "c");
}

#[test]
fn serializes_camel_case_strings() {
    let raw = serde_json::to_string(&UsageLimit {
        id: "s".into(),
        label: "Session".into(),
        scope: Some(console_proto::UsageScope {
            provider: "c".into(),
            ..Default::default()
        }),
        amount: Some(console_proto::UsageAmount {
            unit: "percent".into(),
            ..Default::default()
        }),
        ..Default::default()
    })
    .expect("encode limit");
    assert!(!raw.contains("modelId"), "unset fields must omit: {raw}");
    assert!(raw.contains(r#""unit":"percent""#), "unexpected encoding: {raw}");
}
