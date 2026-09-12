//! Port Forwarding Service for querying, adding, and removing dev server ports.

use crate::types::{ApiResponse, ForwardPortRequest, ForwardedPort};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct PortService {
    transport: HttpTransport,
}

impl PortService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// List all currently available forwarded ports, optionally filtered by project ID.
    pub async fn list(&self, project_id: Option<&str>) -> Result<Vec<ForwardedPort>> {
        let endpoint = if let Some(pid) = project_id {
            format!("/api/ports?projectId={}", urlencoding::encode(pid))
        } else {
            "/api/ports".to_string()
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch forwarded ports")?;

        let body: ApiResponse<Vec<ForwardedPort>> = response
            .json()
            .await
            .context("Failed to parse ports list")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to fetch forwarded ports".into()
            })))
        }
    }

    /// Manually forward a port on the server.
    pub async fn forward(&self, port: u16, project_id: Option<&str>) -> Result<ForwardedPort> {
        let url = self.transport.url("/api/ports/forward").await;
        let req = ForwardPortRequest {
            port,
            project_id: project_id.map(String::from),
        };
        let response = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await)
            .json(&req)
            .send()
            .await
            .context("Failed to forward port")?;

        let body: ApiResponse<ForwardedPort> = response
            .json()
            .await
            .context("Failed to parse forward port response")?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("Port forward response contained no data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to forward port".into())
            ))
        }
    }

    /// Remove a forwarded port.
    pub async fn unforward(&self, port: u16, project_id: Option<&str>) -> Result<()> {
        let endpoint = if let Some(pid) = project_id {
            format!("/api/ports/{}?projectId={}", port, urlencoding::encode(pid))
        } else {
            format!("/api/ports/{}", port)
        };
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .delete(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to unforward port")?;

        let body: ApiResponse<serde_json::Value> = response
            .json()
            .await
            .context("Failed to parse unforward response")?;
        if body.success {
            Ok(())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to unforward port".into())
            ))
        }
    }
}
