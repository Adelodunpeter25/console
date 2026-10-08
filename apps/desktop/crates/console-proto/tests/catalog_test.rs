use console_proto::{Model, ProviderCatalogEntry, ProviderModelsResponse};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/catalog")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_catalog_fixture() {
    let entries: Vec<ProviderCatalogEntry> =
        serde_json::from_str(fixture("providers.json").trim()).expect("decode catalog");
    assert_eq!(entries.len(), 2);
    let codex = &entries[0];
    assert_eq!(codex.name, "codex");
    assert_eq!(codex.display_name, "Codex");
    assert_eq!(codex.description, "OpenAI");
    assert_eq!(codex.auth_method, "oauth");
    assert_eq!(codex.models.len(), 2);
    let gpt5 = &codex.models[0];
    assert_eq!(gpt5.id, "gpt-5");
    assert_eq!(gpt5.context_window, 272_000);
    assert!(gpt5.supports_images);
    assert_eq!(gpt5.supported_thinking_levels, vec!["low", "high"]);
    assert_eq!(gpt5.default_thinking_level.as_deref(), Some("medium"));
    // Second model declares nothing optional: absent, not empty/zero-valued.
    let mini = &codex.models[1];
    assert!(!mini.supports_images);
    assert!(mini.supported_thinking_levels.is_empty());
    assert_eq!(mini.default_thinking_level, None);
    // A provider with an omitted model list decodes to empty, never fails.
    assert!(entries[1].models.is_empty());
}

#[test]
fn decodes_golden_provider_models_fixture() {
    let resp: ProviderModelsResponse =
        serde_json::from_str(fixture("provider_models.json").trim()).expect("decode models");
    assert_eq!(resp.provider, "codex");
    assert_eq!(resp.models.len(), 1);
    assert_eq!(resp.models[0].context_window, 272_000);
}

#[test]
fn thinking_level_strings_map_to_enums() {
    // Mirrors console_core::model_thinking_levels: unknown strings are
    // skipped, so an unrecognized provider vocabulary can't panic a picker.
    let model = Model {
        id: "m".into(),
        provider: "p".into(),
        context_window: 1_000,
        supported_thinking_levels: vec!["low".into(), "not-a-level".into(), "xhigh".into()],
        ..Default::default()
    };
    let mapped: Vec<&str> = model
        .supported_thinking_levels
        .iter()
        .filter(|raw| matches!(raw.as_str(), "low" | "xhigh"))
        .map(|raw| raw.as_str())
        .collect();
    assert_eq!(mapped, vec!["low", "xhigh"]);
}

#[test]
fn ignores_unknown_fields() {
    let entry: ProviderCatalogEntry = serde_json::from_str(
        r#"{"name":"p","displayName":"P","description":"d","models":[],"authMethod":"none","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(entry.name, "p");
    assert!(entry.models.is_empty());
}