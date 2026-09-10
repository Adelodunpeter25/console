use crate::types::{ApiResponse, ConsoleSettings};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct SettingsService {
    transport: HttpTransport,
}

impl SettingsService {
    pub fn new(transport: HttpTransport) -> Self { Self { transport } }

    pub async fn get(&self) -> Result<ConsoleSettings> {
        let url = self.transport.url("/api/settings").await;
        let response = self.transport.client().get(url).headers(self.transport.build_headers().await).send().await.context("Failed to fetch settings")?;
        let body: ApiResponse<ConsoleSettings> = response.json().await.context("Failed to parse settings")?;
        if body.success { Ok(body.data.unwrap_or_default()) } else { Err(anyhow!(body.error.unwrap_or_else(|| "Failed to fetch settings".into()))) }
    }

    pub async fn update(&self, settings: &ConsoleSettings) -> Result<ConsoleSettings> {
        let url = self.transport.url("/api/settings").await;
        let response = self.transport.client().patch(url).headers(self.transport.build_headers().await).json(settings).send().await.context("Failed to save settings")?;
        let body: ApiResponse<ConsoleSettings> = response.json().await.context("Failed to parse saved settings")?;
        if body.success { Ok(body.data.unwrap_or_default()) } else { Err(anyhow!(body.error.unwrap_or_else(|| "Failed to save settings".into()))) }
    }
}
