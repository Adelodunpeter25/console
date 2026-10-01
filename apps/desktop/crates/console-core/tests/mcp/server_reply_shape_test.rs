//! Wire-shape coverage for MCP list/save responses.
//!
//! The desktop client and the Go server disagree in three places, and none of
//! them is a reason for a save or a list refresh to fail:
//!
//! - Every route wraps its payload in `{"success": ..., "data": ...}`. The
//!   service unwraps it, so these tests decode the same envelope.
//! - `name` here vs `label` there; `env` as a pair array here vs an object
//!   there; `auth_type` as a flat string here vs a nested `auth` object.
//! - The list entry carries status as a bare string with any message in a
//!   separate `error` member, while this client encodes `Error` as an object.

use console_core::types::mcp::{
    McpAuthType, McpConnectionStatus, McpServerConfig, McpTransportType,
};
use console_core::types::ApiResponse;

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

/// The envelope is the actual HTTP body — the bare config only ever appears
/// nested inside it. Decoding the unwrapped shape is what produced
/// "failed to decode MCP server response" on every save.
#[test]
fn unwraps_the_save_envelope() {
    let envelope = format!(
        r#"{{"success": true, "data": {SERVER_REPLY}}}"#
    );
    let body: ApiResponse<McpServerConfig> = serde_json::from_str(&envelope)
        .unwrap_or_else(|e| panic!("save envelope must decode: {e}"));
    assert!(body.success);
    let cfg = body.data.expect("saved server must arrive in `data`");
    assert_eq!(cfg.name, "Atlassian");
    assert_eq!(cfg.auth_type, McpAuthType::Static);
}

/// `GET /api/mcp/servers` entries: the stored config (read back by the edit
/// form) plus the live status and tools the row renders. See
/// `apps/server-go/internal/services/mcp/manager.go`.
const LIST_ENTRY: &str = r#"{
  "id": "atlassian",
  "label": "Atlassian",
  "transport": "http",
  "url": "https://mcp.example.com/mcp",
  "env": {"REGION": "eu"},
  "auth": {"type": "static", "tokenRef": "atlassian"},
  "enabled": true,
  "createdAt": 1750000000000,
  "updatedAt": 1750000000000,
  "status": "connected",
  "toolCount": 2,
  "tools": [
    {"name": "get_echo", "description": "Echo", "inputSchema": {}, "readOnly": true},
    {"name": "put_echo", "description": "Echo", "inputSchema": {}, "readOnly": true}
  ]
}"#;

#[test]
fn list_entries_carry_config_status_and_tools() {
    let envelope = format!(r#"{{"success": true, "data": [{LIST_ENTRY}]}}"#);
    let body: ApiResponse<Vec<McpServerConfig>> = serde_json::from_str(&envelope)
        .unwrap_or_else(|e| panic!("list envelope must decode: {e}"));
    let list = body.data.expect("list must arrive in `data`");
    assert_eq!(list.len(), 1);
    let cfg = &list[0];

    // The edit form reads these back before re-saving; a summary-shaped entry
    // would blank them.
    assert_eq!(cfg.name, "Atlassian");
    assert_eq!(cfg.url.as_deref(), Some("https://mcp.example.com/mcp"));
    assert_eq!(cfg.env, vec![("REGION".to_string(), "eu".to_string())]);
    assert_eq!(cfg.auth_type, McpAuthType::Static);

    // What the row renders.
    assert_eq!(cfg.status, McpConnectionStatus::Connected);
    assert_eq!(cfg.tools.len(), 2, "toolCount is derived; tools are the source");
    assert_eq!(cfg.tools[0].name, "get_echo");
}

#[test]
fn bare_error_status_uses_the_server_message() {
    let cfg: McpServerConfig = serde_json::from_str(
        r#"{"id":"a","name":"A","transport":"http",
            "status":"error","error":"connection refused"}"#,
    )
    .expect("a bare error status must decode");
    assert_eq!(cfg.status, McpConnectionStatus::Error("connection refused".into()));
}

#[test]
fn bare_error_without_a_message_stays_readable() {
    // The server omits `error` when it has nothing to say; a blank chip would
    // be worse than a short generic one.
    let cfg: McpServerConfig = serde_json::from_str(
        r#"{"id":"a","name":"A","transport":"http","status":"error"}"#,
    )
    .expect("decode");
    assert_eq!(cfg.status, McpConnectionStatus::Error("connection failed".into()));
}

#[test]
fn this_clients_error_encoding_still_decodes() {
    // The client writes `Error` as `{"error": "..."}`; it must read its own
    // output back.
    let cfg: McpServerConfig = serde_json::from_str(
        r#"{"id":"a","name":"A","transport":"http","status":{"error":"boom"}}"#,
    )
    .expect("decode");
    assert_eq!(cfg.status, McpConnectionStatus::Error("boom".into()));
}

#[test]
fn an_unrecognised_status_does_not_break_the_list() {
    // One server in a state this build does not know about must not blank
    // every other entry in the settings list.
    let envelope = format!(
        r#"{{"success": true, "data": [
            {{"id":"ok","label":"Ok","transport":"http","status":"connected"}},
            {{"id":"odd","label":"Odd","transport":"http","status":"warming_up"}}
        ]}}"#
    );
    let body: ApiResponse<Vec<McpServerConfig>> =
        serde_json::from_str(&envelope).expect("decode list");
    let list = body.data.expect("data");
    assert_eq!(list.len(), 2);
    assert_eq!(list[0].status, McpConnectionStatus::Connected);
    assert_eq!(list[1].status, McpConnectionStatus::Error("warming_up".into()));
}
