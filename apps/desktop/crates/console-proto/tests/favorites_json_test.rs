use console_proto::{ModelFavorite, SetFavoriteResponse};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/favorites")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_list_fixture() {
    let items: Vec<ModelFavorite> =
        serde_json::from_str(fixture("list.json").trim()).expect("decode list fixture");
    assert_eq!(items.len(), 1);
    assert_eq!(items[0].provider, "anthropic");
    assert_eq!(items[0].model_id, "claude-opus-4");
}

#[test]
fn decodes_golden_set_response_fixture() {
    let msg: SetFavoriteResponse =
        serde_json::from_str(fixture("set_response.json").trim()).expect("decode set fixture");
    assert_eq!(msg.provider, "anthropic");
    assert_eq!(msg.model_id, "claude-opus-4");
    assert!(msg.favorite);
}

#[test]
fn ignores_unknown_fields() {
    let items: Vec<ModelFavorite> = serde_json::from_str(
        r#"[{"provider":"a","modelId":"b","futureField":"x"}]"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(items[0].provider, "a");
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&ModelFavorite {
        provider: "a".into(),
        model_id: "b".into(),
    })
    .expect("encode favorite");
    assert!(raw.contains("modelId"), "unexpected encoding: {raw}");
}
