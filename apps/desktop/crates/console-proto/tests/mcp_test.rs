use console_proto::{McpServerConfig, McpServerDetail, McpServerStatus, McpTool};
use std::path::PathBuf;

fn fixture(name: &str) -> String {
    std::fs::read_to_string(
        PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../../..")
            .join("proto/testdata/mcp")
            .join(name),
    )
    .unwrap_or_else(|e| panic!("read {name}: {e}"))
}

#[test]
fn decodes_golden_servers_fixture() {
    let rows: Vec<McpServerStatus> =
        serde_json::from_str(fixture("servers.json").trim()).expect("decode servers");
    assert_eq!(rows.len(), 2);
    let full = &rows[0];
    assert_eq!(full.id, "atlassian");
    assert_eq!(full.label, "Atlassian");
    assert_eq!(full.transport.as_deref(), Some("http"));
    assert_eq!(full.url.as_deref(), Some("https://mcp.example"));
    assert_eq!(full.env.get("REGION").map(String::as_str), Some("eu"));
    let auth = full.auth.as_ref().expect("auth present");
    assert_eq!(auth.r#type, "static");
    assert_eq!(auth.token_ref.as_deref(), Some("atlassian"));
    assert_eq!(
        full.tier_overrides.get("read").map(String::as_str),
        Some("write")
    );
    assert_eq!(full.enabled, Some(true));
    // int64 timestamps arrive as protojson strings (unread client-side).
    assert_eq!(full.created_at, 1700000000000);
    assert_eq!(full.status, "connected");
    assert_eq!(full.tool_count, 2);
    assert_eq!(full.tools.len(), 2);
    assert_eq!(full.tools[0].name, "t1");
    assert_eq!(full.tools[0].description.as_deref(), Some("d1"));
    assert_eq!(full.tools[1].description, None);

    // Minimal stdio row: absent keys decode to defaults, explicit
    // enabled:false stays present (never confused with absent).
    let minimal = &rows[1];
    assert_eq!(minimal.id, "local");
    assert_eq!(minimal.label, "");
    assert_eq!(minimal.transport.as_deref(), Some("stdio"));
    assert_eq!(minimal.command.as_deref(), Some("npx"));
    assert_eq!(minimal.args, vec!["-y".to_string(), "pkg".to_string()]);
    assert_eq!(minimal.enabled, Some(false));
    assert_eq!(minimal.status, "disconnected");
    assert!(minimal.tools.is_empty());
}

#[test]
fn decodes_golden_detail_fixture() {
    let detail: McpServerDetail =
        serde_json::from_str(fixture("server_detail.json").trim()).expect("decode detail");
    let config: McpServerConfig = detail.config.expect("detail config");
    assert_eq!(config.id, "atlassian");
    assert_eq!(config.enabled, Some(true));
    assert_eq!(detail.tools.len(), 1);
    let tool: &McpTool = &detail.tools[0];
    assert_eq!(tool.name, "t1");
}

#[test]
fn ignores_unknown_fields() {
    let row: McpServerStatus = serde_json::from_str(
        r#"{"id":"s","label":"S","transport":"stdio","enabled":true,"status":"disconnected","futureField":1}"#,
    )
    .expect("unknown fields must be ignored");
    assert_eq!(row.id, "s");
}
