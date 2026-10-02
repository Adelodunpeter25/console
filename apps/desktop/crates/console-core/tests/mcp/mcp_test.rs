use console_core::types::mcp::{
    McpAuthType, McpConnectionStatus, McpServerConfig, McpToolInfo, McpTransportType,
};

#[test]
fn test_mcp_config_serialization() {
    let config = McpServerConfig {
        id: "atlassian".to_string(),
        name: "Atlassian MCP".to_string(),
        transport: McpTransportType::Http,
        url: Some("https://mcp.atlassian.com/v2/mcp".to_string()),
        auth_type: McpAuthType::Oauth2,
        command: None,
        args: vec![],
        env: vec![],
        status: McpConnectionStatus::Connected,
        auth_url: None,
        tools: vec![McpToolInfo {
            name: "jira_search".to_string(),
            description: Some("Search Jira issues".to_string()),
        }],
    };

    let serialized = serde_json::to_string(&config).expect("failed to serialize MCP config");
    let deserialized: McpServerConfig =
        serde_json::from_str(&serialized).expect("failed to deserialize MCP config");

    assert_eq!(deserialized.id, "atlassian");
    assert_eq!(deserialized.transport, McpTransportType::Http);
    assert_eq!(deserialized.auth_type, McpAuthType::Oauth2);
    assert_eq!(deserialized.tools.len(), 1);
    assert_eq!(deserialized.tools[0].name, "jira_search");
}

#[test]
fn test_mcp_stdio_config() {
    let config = McpServerConfig {
        id: "filesystem".to_string(),
        name: "Local Filesystem".to_string(),
        transport: McpTransportType::Stdio,
        url: None,
        auth_type: McpAuthType::None,
        command: Some("npx".to_string()),
        args: vec!["-y".to_string(), "@modelcontextprotocol/server-filesystem".to_string()],
        env: vec![("ROOT_DIR".to_string(), "/tmp".to_string())],
        status: McpConnectionStatus::Disconnected,
        auth_url: None,
        tools: vec![],
    };

    let serialized = serde_json::to_string(&config).expect("failed to serialize stdio config");
    let deserialized: McpServerConfig =
        serde_json::from_str(&serialized).expect("failed to deserialize stdio config");

    assert_eq!(deserialized.id, "filesystem");
    assert_eq!(deserialized.transport, McpTransportType::Stdio);
    assert_eq!(deserialized.command.as_deref(), Some("npx"));
    assert_eq!(deserialized.args.len(), 2);
    assert_eq!(deserialized.env.len(), 1);
}
