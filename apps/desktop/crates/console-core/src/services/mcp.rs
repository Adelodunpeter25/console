use crate::types::mcp::{McpServerConfig, McpToolInfo};
use crate::types::ApiResponse;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

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
        let body: ApiResponse<Vec<McpServerConfig>> = self
            .transport
            .decode_json(resp)
            .await
            .context("failed to decode MCP servers response")?;
        if !body.success {
            anyhow::bail!(body
                .error
                .unwrap_or_else(|| "failed to list MCP servers".into()));
        }
        // An empty list is not an error; the settings page just renders it.
        Ok(body.data.unwrap_or_default())
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
        let body: ApiResponse<McpServerConfig> = self
            .transport
            .decode_json(resp)
            .await
            .context("failed to decode MCP server response")?;
        if !body.success {
            anyhow::bail!(body
                .error
                .unwrap_or_else(|| "failed to save MCP server".into()));
        }
        body.data
            .ok_or_else(|| anyhow!("MCP server save returned no server"))
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

    /// Connect, then poll until the server settles. The route only
    /// acknowledges the request (`{"started":true}`) and finishes in the
    /// background, so a single refresh would leave the chip stuck on
    /// "connecting" until the settings tab is reopened. Returns the refreshed
    /// list for the caller to apply.
    /// Starts a connect and returns once the request is accepted, without
    /// waiting for the flow to settle. When `redirect_uri` is given the
    /// OAuth callback lands on the caller's loopback and the caller opens
    /// the browser plus forwards the result; without it the server keeps
    /// its loopback behavior. Callers poll `list_servers` for progress.
    pub async fn start_connect(&self, id: &str, redirect_uri: Option<&str>) -> Result<()> {
        let url = self.transport.url(&format!("/api/mcp/servers/{}/connect", id)).await;
        let mut req = self.transport.client().post(&url);
        if let Some(redirect_uri) = redirect_uri {
            req = req.json(&serde_json::json!({ "redirectUri": redirect_uri }));
        }
        let resp = req.send().await.context("failed to connect MCP server")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to connect MCP server: {} {}", status, body);
        }
        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("failed to decode connect MCP response")?;
        if !body.success {
            anyhow::bail!(body
                .error
                .unwrap_or_else(|| "failed to connect MCP server".into()));
        }
        Ok(())
    }

    /// Forwards a code+state captured on the caller's own loopback listener
    /// into the pending server-initiated OAuth flow.
    pub async fn forward_oauth_callback(
        &self,
        id: &str,
        state: &str,
        code: Option<&str>,
        error: Option<&str>,
        iss: Option<&str>,
    ) -> Result<()> {
        let url = self
            .transport
            .url(&format!("/api/mcp/servers/{}/oauth/callback", id))
            .await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .json(&serde_json::json!({
                "state": state,
                "code": code,
                "error": error,
                "iss": iss,
            }))
            .send()
            .await
            .context("failed to forward MCP OAuth callback")?;
        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().await.unwrap_or_default();
            anyhow::bail!("failed to forward MCP OAuth callback: {} {}", status, body);
        }
        let body: ApiResponse<serde_json::Value> = self
            .transport
            .decode_json(resp)
            .await
            .context("failed to decode MCP OAuth callback response")?;
        if !body.success {
            anyhow::bail!(body
                .error
                .unwrap_or_else(|| "failed to forward MCP OAuth callback".into()));
        }
        Ok(())
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
