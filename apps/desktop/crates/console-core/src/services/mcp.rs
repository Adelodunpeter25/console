use crate::types::mcp::{McpServerConfig, McpToolInfo};
use crate::utils::HttpTransport;
use anyhow::{Context, Result};

#[derive(Clone)]
pub struct McpService {
    transport: HttpTransport,
}

impl McpService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list_servers(&self) -> Result<Vec<McpServerConfig>> {
        let url = self.transport.url("/api/mcp/servers").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .send()
            .await
            .context("failed to list MCP servers")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to list MCP servers: {} {}", status, body);
        }
        resp.json().await.context("failed to decode MCP servers response")
    }

    pub async fn save_server(&self, server: &McpServerConfig) -> Result<McpServerConfig> {
        let url = self.transport.url("/api/mcp/servers").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .json(server)
            .send()
            .await
            .context("failed to save MCP server")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to save MCP server: {} {}", status, body);
        }
        resp.json().await.context("failed to decode MCP server response")
    }

    pub async fn delete_server(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/mcp/servers/{}", id)).await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .send()
            .await
            .context("failed to delete MCP server")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to delete MCP server: {} {}", status, body);
        }
        Ok(())
    }

    pub async fn connect_server(&self, id: &str) -> Result<McpServerConfig> {
        let url = self.transport.url(&format!("/api/mcp/servers/{}/connect", id)).await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .send()
            .await
            .context("failed to connect MCP server")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to connect MCP server: {} {}", status, body);
        }
        resp.json().await.context("failed to decode connect MCP response")
    }

    pub async fn disconnect_server(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/mcp/servers/{}/disconnect", id)).await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .send()
            .await
            .context("failed to disconnect MCP server")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to disconnect MCP server: {} {}", status, body);
        }
        Ok(())
    }

    pub async fn list_tools(&self, id: &str) -> Result<Vec<McpToolInfo>> {
        let url = self.transport.url(&format!("/api/mcp/servers/{}/tools", id)).await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .send()
            .await
            .context("failed to list MCP tools")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to list MCP tools: {} {}", status, body);
        }
        resp.json().await.context("failed to decode MCP tools response")
    }
}
