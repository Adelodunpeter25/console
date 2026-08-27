//! Client for `/api/usage` and `/api/providers/:id/usage` endpoints.

use std::collections::HashMap;
use crate::types::{ApiResponse, UsageReport};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct UsageService {
    transport: HttpTransport,
}

impl UsageService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    /// `GET /api/usage` — Fetch usage reports for all providers.
    pub async fn get_all(&self) -> Result<HashMap<String, Option<UsageReport>>> {
        let url = self.transport.url("/api/usage").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch all usage reports")?;

        let body: ApiResponse<HashMap<String, Option<UsageReport>>> = resp
            .json()
            .await
            .context("Failed to parse usage response")?;

        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to fetch usage reports".into())
            ))
        }
    }

    /// `GET /api/providers/:id/usage` — Fetch usage report for a single provider.
    pub async fn get_provider(&self, provider_id: &str) -> Result<Option<UsageReport>> {
        let url = self
            .transport
            .url(&format!("/api/providers/{provider_id}/usage"))
            .await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch provider usage")?;

        let body: ApiResponse<Option<UsageReport>> = resp
            .json()
            .await
            .context("Failed to parse provider usage response")?;

        if body.success {
            Ok(body.data.flatten())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to fetch provider usage".into())
            ))
        }
    }
}
