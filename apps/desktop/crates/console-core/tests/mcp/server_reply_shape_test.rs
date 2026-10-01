//! `McpServerConfig` has to survive both directions of the wire, because the
//! desktop client and the Go server spell three fields differently.
//!
//! - `name` here vs `label` on the server.
//! - `env` as a pair array here vs an object on the server.
//! - `auth_type` as a flat string here vs a nested `auth` object there.
//!
//! A shape difference is not a reason for a save or a list refresh to fail.

use console_core::types::mcp::{McpAuthType, McpServerConfig, McpTransportType};

/// The JSON the Go server returns from `POST /api/mcp/servers` — a saved
/// `ServerConfig`. Keys and shapes come from
/// `apps/server-go/internal/services/mcp/config.go`.
const SERVER_REPLY: &str = r#"{
  "id": "atlassian",
  "label": "Atlassian",
  "transport": "http",
  "url": "https://mcp.example.com/mcp",
  "env": {"TOKEN": "abc", "REGION": "eu"},
  "auth": {"type": "static", "tokenRef": "atlassian"},
  "enabled": true,
  "createdAt": 1750000000000,
  "updatedAt": 1750000000000
}"#;

#[test]
fn decodes_server_reply_into_client_type() {
    let cfg: McpServerConfig = serde_json::from_str(SERVER_REPLY)
        .unwrap_or_else(|e| panic!("server reply must decode: {e}"));
    assert_eq!(cfg.id, "atlassian");
    assert_eq!(cfg.name, "Atlassian", "label must populate name");
    assert_eq!(cfg.transport, McpTransportType::Http);
    assert_eq!(cfg.env, vec![
        ("TOKEN".to_string(), "abc".to_string()),
        ("REGION".to_string(), "eu".to_string()),
    ]);
    // Without this the settings panel would report "no auth" for a server the
    // user explicitly configured a token for.
    assert_eq!(cfg.auth_type, McpAuthType::Static);
}

#[test]
fn still_decodes_the_shape_this_client_sends() {
    let cfg: McpServerConfig = serde_json::from_str(
        r#"{"id":"a","name":"A","transport":"stdio","auth_type":"oauth2",
            "env":[["K","V"]]}"#,
    )
    .unwrap_or_else(|e| panic!("client shape must still decode: {e}"));
    assert_eq!(cfg.name, "A");
    assert_eq!(cfg.auth_type, McpAuthType::Oauth2);
    assert_eq!(cfg.env, vec![("K".to_string(), "V".to_string())]);
}

#[test]
fn env_object_order_is_preserved() {
    // The settings list renders env as "K=V, K=V"; unstable ordering would
    // make the text churn between frames.
    let cfg: McpServerConfig = serde_json::from_str(
        r#"{"id":"a","name":"A","transport":"http","env":{"ZED":"1","ALPHA":"2","MID":"3"}}"#,
    )
    .expect("decode");
    let keys: Vec<&str> = cfg.env.iter().map(|(k, _)| k.as_str()).collect();
    assert_eq!(keys, vec!["ZED", "ALPHA", "MID"]);
}

#[test]
fn missing_auth_defaults_to_none() {
    let cfg: McpServerConfig =
        serde_json::from_str(r#"{"id":"a","name":"A","transport":"http"}"#).expect("decode");
    assert_eq!(cfg.auth_type, McpAuthType::None);
    assert!(cfg.env.is_empty());
}

#[test]
fn round_trips_through_the_server_canonical_shape() {
    // Whatever this client posts must be decodable back into the same type,
    // which is what the save path does before applying its own response.
    let original: McpServerConfig = serde_json::from_str(SERVER_REPLY).expect("decode reply");
    let posted = serde_json::to_string(&original).expect("encode");
    let again: McpServerConfig =
        serde_json::from_str(&posted).unwrap_or_else(|e| panic!("re-decode {posted}: {e}"));
    assert_eq!(again.name, original.name);
    assert_eq!(again.env, original.env);
    assert_eq!(again.auth_type, original.auth_type);
}
