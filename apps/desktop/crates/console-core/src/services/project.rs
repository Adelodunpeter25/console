use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct ProjectService {
    transport: HttpTransport,
}

impl ProjectService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self) -> Result<Vec<ProjectInfo>> {
        let url = self.transport.url("/api/projects").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list projects")?;

        let body: ApiResponse<Vec<ProjectInfo>> = resp
            .json()
            .await
            .context("Failed to parse projects response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list projects".into())
            ))
        }
    }

    pub async fn add(&self, path: &str) -> Result<ProjectInfo> {
        let url = self.transport.url("/api/projects").await;
        let payload = serde_json::json!({ "path": path });

        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to add project")?;

        let body: ApiResponse<ProjectInfo> = resp
            .json()
            .await
            .context("Failed to parse add project response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Project data is missing"))
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to add project".into())
            ))
        }
    }

    /// `DELETE /api/projects/:id` — remove a project workspace by id.
    pub async fn remove(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/projects/{}", id)).await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to remove project")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse remove project response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to remove project".into())
            ))
        }
    }
}
