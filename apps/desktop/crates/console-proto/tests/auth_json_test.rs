use console_proto::{
    AuthStatusResponse, GitHubAuthStatus, OAuthCallbackResponse, OAuthLoginUrlResponse,
    ProjectIdResponse,
};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/auth")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_status_fixture() {
    let msg: AuthStatusResponse =
        serde_json::from_str(fixture("status.json").trim()).expect("decode status");
    let antigravity = msg.antigravity.as_ref().expect("antigravity reported");
    assert!(antigravity.logged_in);
    assert_eq!(antigravity.email.as_deref(), Some("adelodunjoseph892@gmail.com"));
    assert!(msg.codex.as_ref().unwrap().logged_in);
    assert!(msg.claude.as_ref().unwrap().logged_in);
    // devin is reserved: reported but never logged in, so a client never has
    // to tell "logged out" apart from "not reported".
    assert!(!msg.devin.as_ref().unwrap().logged_in);
    assert!(!msg.github.as_ref().unwrap().connected);
}

#[test]
fn decodes_golden_status_logged_out_fixture() {
    let msg: AuthStatusResponse =
        serde_json::from_str(fixture("status_logged_out.json").trim()).expect("decode status");
    // Every provider is still present, just empty.
    assert!(!msg.antigravity.as_ref().unwrap().logged_in);
    assert!(!msg.codex.as_ref().unwrap().logged_in);
    assert!(!msg.claude.as_ref().unwrap().logged_in);
    assert!(!msg.devin.as_ref().unwrap().logged_in);
    assert!(msg.antigravity.as_ref().unwrap().email.is_none());
}

#[test]
fn decodes_golden_login_url_fixture() {
    let msg: OAuthLoginUrlResponse =
        serde_json::from_str(fixture("login_url.json").trim()).expect("decode login url");
    assert_eq!(msg.provider, "codex");
    assert!(msg.auth_url.contains("state=st_123"));
    assert_eq!(msg.state, "st_123");
    assert!(msg.redirect_uri.ends_with("/auth/callback"));
}

#[test]
fn decodes_golden_callback_fixture() {
    let msg: OAuthCallbackResponse =
        serde_json::from_str(fixture("callback.json").trim()).expect("decode callback");
    assert_eq!(msg.provider, "codex");
    assert_eq!(msg.user_email.as_deref(), Some("user@example.com"));
}

// An unset project id used to arrive as null; it is now simply absent. Both
// read as None, which is the point of the trim.
#[test]
fn decodes_golden_project_id_fixtures() {
    let unset: ProjectIdResponse = serde_json::from_str(fixture("project_id_unset.json").trim())
        .expect("decode unset project id");
    assert!(unset.project_id.is_none());

    let set: ProjectIdResponse = serde_json::from_str(fixture("project_id_set.json").trim())
        .expect("decode set project id");
    assert_eq!(set.project_id.as_deref(), Some("my-gcp-project"));
}

#[test]
fn decodes_golden_github_status_fixture() {
    let msg: GitHubAuthStatus =
        serde_json::from_str(fixture("github_status.json").trim()).expect("decode github status");
    assert!(msg.connected);
    assert_eq!(msg.username.as_deref(), Some("adelodunpeter"));
    assert_eq!(msg.scopes, vec!["repo".to_string(), "workflow".to_string()]);
}

#[test]
fn ignores_unknown_fields() {
    let msg: AuthStatusResponse = serde_json::from_str(
        r#"{"codex":{"loggedIn":true,"futureField":"x"},"futureTop":"y"}"#,
    )
    .expect("unknown fields must be ignored");
    assert!(msg.codex.as_ref().unwrap().logged_in);
}

#[test]
fn serializes_camel_case() {
    let raw = serde_json::to_string(&OAuthLoginUrlResponse {
        provider: "codex".into(),
        auth_url: "https://x".into(),
        state: "s".into(),
        redirect_uri: "http://localhost:1455/auth/callback".into(),
    })
    .expect("encode login url");
    assert!(raw.contains(r#""authUrl":"https://x""#), "unexpected encoding: {raw}");
    assert!(raw.contains(r#""redirectUri""#), "unexpected encoding: {raw}");
}