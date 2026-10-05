use crate::types::{ApiResponse, ConsoleSettings};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};
use serde::Serialize;

/// PATCH request body. Request-only: a null role clears it server-side while
/// a missing key leaves it untouched, so None must serialize as JSON null
/// (never be omitted) — the opposite of protojson. This mirrors the old
/// hand-written ConsoleSettings encoding byte-for-byte.
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PatchSettingsBody {
    pub model_roles: PatchRoleMapping,
}

#[derive(Serialize)]
pub struct PatchRoleMapping {
    pub vision: Option<String>,
    pub smol: Option<String>,
}

impl PatchSettingsBody {
    pub fn from_settings(settings: &ConsoleSettings) -> Self {
        let roles = settings.model_roles.as_ref();
        Self {
            model_roles: PatchRoleMapping {
                vision: roles.and_then(|r| r.vision.clone()),
                smol: roles.and_then(|r| r.smol.clone()),
            },
        }
    }
}

#[derive(Clone)]
pub struct SettingsService {
    transport: HttpTransport,
}

impl SettingsService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    pub async fn get(&self) -> Result<ConsoleSettings> {
        let url = self.transport.url("/api/settings").await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to fetch settings")?;
        let body: ApiResponse<ConsoleSettings> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse settings")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to fetch settings".into())
            ))
        }
    }

    pub async fn update(&self, settings: &ConsoleSettings) -> Result<ConsoleSettings> {
        let url = self.transport.url("/api/settings").await;
        let response = self
            .transport
            .client()
            .patch(url)
            .headers(self.transport.build_headers().await)
            .json(&PatchSettingsBody::from_settings(settings))
            .send()
            .await
            .context("Failed to save settings")?;
        let body: ApiResponse<ConsoleSettings> = self
            .transport
            .decode_json(response)
            .await
            .context("Failed to parse saved settings")?;
        if body.success {
            Ok(body.data.unwrap_or_default())
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| "Failed to save settings".into())
            ))
        }
    }
}
