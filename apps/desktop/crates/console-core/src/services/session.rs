use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use std::time::Duration;

#[derive(Clone)]
pub struct SessionService {
    transport: HttpTransport,
}

impl SessionService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(
        &self,
        cwd: Option<&str>,
        project_id: Option<&str>,
    ) -> Result<Vec<SessionHeader>> {
        let mut url = self.transport.url("/api/sessions").await;
        let mut query = Vec::new();
        if let Some(c) = cwd {
            query.push(format!("cwd={}", urlencoding::encode(c)));
        }
        if let Some(p) = project_id {
            query.push(format!("projectId={}", urlencoding::encode(p)));
        }
        if !query.is_empty() {
            url.push('?');
            url.push_str(&query.join("&"));
        }

        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list sessions")?;

        let body: ApiResponse<Vec<SessionHeader>> = resp
            .json()
            .await
            .context("Failed to parse sessions response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list sessions".into())
            ))
        }
    }

    pub async fn get(&self, id: &str) -> Result<SessionDetailResponse> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to get session")?;

        let body: ApiResponse<SessionDetailResponse> = resp
            .json()
            .await
            .context("Failed to parse session response")?;
        if body.success {
            body.data.ok_or_else(|| anyhow!("Session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to load session".into())
            ))
        }
    }

    /// Reload a session until the backend has finished persisting an active run.
    /// This is used after an SSE disconnect so the UI does not immediately
    /// submit another prompt against a still-running server session.
    pub async fn wait_until_settled(&self, id: &str) -> Result<SessionDetailResponse> {
        const MAX_ATTEMPTS: usize = 120;
        const POLL_INTERVAL: Duration = Duration::from_millis(250);

        for attempt in 0..MAX_ATTEMPTS {
            let detail = self.get(id).await?;
            if detail.header.status != Some(SessionStatus::Working) || attempt + 1 == MAX_ATTEMPTS {
                return Ok(detail);
            }
            tokio::time::sleep(POLL_INTERVAL).await;
        }

        unreachable!("session settlement loop always returns");
    }

    pub async fn create(&self, payload: CreateSessionDto) -> Result<SessionHeader> {
        let url = self.transport.url("/api/sessions").await;
        let resp = self
            .transport
            .client()
            .post(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to create session")?;

        let body: ApiResponse<SessionHeader> = resp
            .json()
            .await
            .context("Failed to parse create session response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Created session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to create session".into())
            ))
        }
    }

    pub async fn update(&self, id: &str, payload: UpdateSessionDto) -> Result<SessionHeader> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let resp = self
            .transport
            .client()
            .patch(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to update session")?;

        let body: ApiResponse<SessionHeader> = resp
            .json()
            .await
            .context("Failed to parse update session response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Updated session data is missing"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to update session".into())
            ))
        }
    }

    pub async fn delete(&self, id: &str) -> Result<()> {
        let url = self.transport.url(&format!("/api/sessions/{}", id)).await;
        let resp = self
            .transport
            .client()
            .delete(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to delete session")?;

        let body: ApiResponse<serde_json::Value> = resp
            .json()
            .await
            .context("Failed to parse delete session response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to delete session".into())
            ))
        }
    }
}
