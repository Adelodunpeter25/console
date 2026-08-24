use anyhow::{Context, Result, anyhow};
use serde::Serialize;

use crate::types::{ApiResponse, ModelFavorite};
use crate::utils::HttpTransport;

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
struct SetModelFavoriteDto {
    provider: String,
    model_id: String,
    favorite: bool,
}

#[derive(Clone)]
pub struct ModelFavoriteService {
    transport: HttpTransport,
}

impl ModelFavoriteService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn list(&self) -> Result<Vec<ModelFavorite>> {
        let url = self.transport.url("/api/model-favorites").await;
        let response = self
            .transport
            .client()
            .get(&url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to list model favorites")?;

        let body: ApiResponse<Vec<ModelFavorite>> = response
            .json()
            .await
            .context("Failed to parse model favorites response")?;

        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to list model favorites".into())
            ))
        }
    }

    pub async fn set(&self, favorite: ModelFavorite, is_favorite: bool) -> Result<()> {
        let url = self.transport.url("/api/model-favorites").await;
        let payload = SetModelFavoriteDto {
            provider: favorite.provider,
            model_id: favorite.model_id,
            favorite: is_favorite,
        };
        let response = self
            .transport
            .client()
            .put(&url)
            .headers(self.transport.build_headers().await)
            .json(&payload)
            .send()
            .await
            .context("Failed to update model favorite")?;

        let body: ApiResponse<serde_json::Value> = response
            .json()
            .await
            .context("Failed to parse model favorite response")?;

        if body.success {
            Ok(())
        } else {
            Err(anyhow!(body.error.unwrap_or_else(|| {
                "Failed to update model favorite".into()
            })))
        }
    }
}
