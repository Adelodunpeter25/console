use crate::types::*;
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct ProviderService {
    transport: HttpTransport,
}

impl ProviderService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self) -> Result<Vec<ProviderCatalogEntry>> {
        let url = self.transport.url("/api/providers").await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list providers")?;

        let body: ApiResponse<Vec<ProviderCatalogEntry>> = resp
            .json()
            .await
            .context("Failed to parse providers response")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list providers".into())
            ))
        }
    }

    pub async fn get_models(&self, provider_id: &str) -> Result<Vec<Model>> {
        let url = self
            .transport
            .url(&format!("/api/providers/{}/models", provider_id))
            .await;
        let resp = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list provider models")?;

        let body: ApiResponse<ProviderModelsResponse> = resp
            .json()
            .await
            .context("Failed to parse provider models response")?;
        if body.success {
            Ok(body.data.map(|d| d.models).unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error.unwrap_or_else(|| "Failed to list models".into())
            ))
        }
    }
}
