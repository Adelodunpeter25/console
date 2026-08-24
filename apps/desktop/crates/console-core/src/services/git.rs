use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct GitService {
    transport: HttpTransport,
}

impl GitService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn get_status(&self, path: Option<&str>) -> Result<GitStatusSummary> {
        let mut url = self.transport.url("/api/git/status").await;
        if let Some(p) = path {
            url.push_str(&format!("?path={}", urlencoding::encode(p)));
        }

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get git status")?;

        let body: ApiResponse<GitStatusSummary> = resp
            .json()
            .await
            .context("Failed to parse git status response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Git status data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to get git status".into())
            ))
        }
    }

    pub async fn list_branches(&self, path: Option<&str>) -> Result<GitBranchesResponse> {
        let mut url = self.transport.url("/api/git/branches").await;
        if let Some(p) = path {
            url.push_str(&format!("?path={}", urlencoding::encode(p)));
        }

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list git branches")?;

        let body: ApiResponse<GitBranchesResponse> = resp
            .json()
            .await
            .context("Failed to parse git branches response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Git branches data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list git branches".into())
            ))
        }
    }

    pub async fn checkout_branch(&self, path: Option<&str>, branch: &str) -> Result<()> {
        let url = self.transport.url("/api/git/checkout").await;
        let payload = serde_json::json!({ "path": path, "branch": branch });

        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to checkout git branch")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse checkout response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to checkout branch".into())
            ))
        }
    }
}
