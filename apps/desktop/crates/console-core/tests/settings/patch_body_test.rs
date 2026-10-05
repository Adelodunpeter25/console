use console_core::services::settings::{PatchRoleMapping, PatchSettingsBody};

#[test]
fn patch_body_preserves_nulls() {
    // Clearing vision must send explicit null (server clears), not omit
    // the key (server would leave it untouched).
    let body = PatchSettingsBody {
        model_roles: PatchRoleMapping {
            vision: None,
            smol: Some("openai/gpt-5-mini".into()),
        },
    };
    let raw = serde_json::to_string(&body).expect("encode patch body");
    assert_eq!(
        raw,
        r#"{"modelRoles":{"vision":null,"smol":"openai/gpt-5-mini"}}"#
    );
}
