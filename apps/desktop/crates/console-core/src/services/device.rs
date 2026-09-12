//! Device simulator service: discovery, lifecycle, and interaction.
//!
//! Thin HTTP client over the server `/api/devices` routes (mirrors
//! `apps/server/api/src/routes/devices.ts`). Live screen streaming is owned
//! by the `expo-device-hub` proxy, not this service (see the device-simulator
//! service spec); this only covers the REST control plane.

use crate::types::{
    ApiResponse, DeviceActionRequest, DeviceDescriptor, DeviceDiagnostics, DeviceOpenAppRequest,
};
use crate::utils::HttpTransport;
use anyhow::{Context, Result, anyhow};

#[derive(Clone)]
pub struct DeviceService {
    transport: HttpTransport,
}

impl DeviceService {
    pub fn new(transport: HttpTransport) -> Self {
        Self { transport }
    }

    async fn get<T: serde::de::DeserializeOwned>(&self, endpoint: &str) -> Result<T> {
        let url = self.transport.url(endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .with_context(|| format!("Failed to reach {endpoint}"))?;
        let body: ApiResponse<T> = response
            .json()
            .await
            .with_context(|| format!("Failed to parse {endpoint} response"))?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("{endpoint} response contained no data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| format!("Request to {endpoint} failed"))
            ))
        }
    }

    async fn post<T: serde::de::DeserializeOwned>(
        &self,
        endpoint: &str,
        payload: Option<serde_json::Value>,
    ) -> Result<T> {
        let url = self.transport.url(endpoint).await;
        let mut request = self
            .transport
            .client()
            .post(url)
            .headers(self.transport.build_headers().await);
        if let Some(payload) = payload {
            request = request.json(&payload);
        }
        let response = request
            .send()
            .await
            .with_context(|| format!("Failed to POST {endpoint}"))?;
        let body: ApiResponse<T> = response
            .json()
            .await
            .with_context(|| format!("Failed to parse {endpoint} response"))?;
        if body.success {
            body.data
                .ok_or_else(|| anyhow!("{endpoint} response contained no data"))
        } else {
            Err(anyhow!(
                body.error
                    .unwrap_or_else(|| format!("Request to {endpoint} failed"))
            ))
        }
    }

    /// List discovered simulators, emulators, and physical devices.
    pub async fn list(&self) -> Result<Vec<DeviceDescriptor>> {
        let devices: Option<Vec<DeviceDescriptor>> = self.get("/api/devices").await.ok();
        Ok(devices.unwrap_or_default())
    }

    /// Run the platform-tools diagnostic check (Xcode, Android SDK, disk).
    pub async fn diagnostics(&self) -> Result<DeviceDiagnostics> {
        self.get("/api/devices/diagnostics").await
    }

    /// Boot a simulator or emulator by id.
    pub async fn boot(&self, id: &str, platform: &str) -> Result<()> {
        let endpoint = format!("/api/devices/{}/boot?platform={}", id, platform);
        let _: Option<serde_json::Value> = self.post(&endpoint, None).await.ok().flatten();
        Ok(())
    }

    /// Power off a device by id.
    pub async fn shutdown(&self, id: &str, platform: &str) -> Result<()> {
        let endpoint = format!("/api/devices/{}/shutdown?platform={}", id, platform);
        let _: Option<serde_json::Value> = self.post(&endpoint, None).await.ok().flatten();
        Ok(())
    }

    /// Power off all running simulators, emulators, and streaming helpers.
    pub async fn shutdown_all(&self) -> Result<()> {
        let _: Option<serde_json::Value> = self.post("/api/devices/shutdown-all", None).await.ok().flatten();
        Ok(())
    }

    /// Install or launch a target bundle/package on a booted device.
    pub async fn open_app(&self, id: &str, platform: &str, app: &str) -> Result<()> {
        let endpoint = format!("/api/devices/{}/open-app?platform={}", id, platform);
        let payload = serde_json::to_value(DeviceOpenAppRequest {
            app: app.to_string(),
        })?;
        let _: Option<serde_json::Value> = self.post(&endpoint, Some(payload)).await.ok().flatten();
        Ok(())
    }

    /// Execute a tap, swipe, keystroke, or hardware-button action.
    pub async fn interact(
        &self,
        id: &str,
        platform: &str,
        action: DeviceActionRequest,
    ) -> Result<()> {
        let endpoint = format!("/api/devices/{}/interact?platform={}", id, platform);
        let payload = serde_json::to_value(action)?;
        let _: Option<serde_json::Value> = self.post(&endpoint, Some(payload)).await.ok().flatten();
        Ok(())
    }

    /// Capture a live screenshot as raw PNG bytes.
    pub async fn screenshot(&self, id: &str, platform: &str) -> Result<Vec<u8>> {
        let endpoint = format!("/api/devices/{}/screenshot?platform={}", id, platform);
        let url = self.transport.url(&endpoint).await;
        let response = self
            .transport
            .client()
            .get(url)
            .headers(self.transport.build_headers().await)
            .send()
            .await
            .context("Failed to capture device screenshot")?;
        if !response.status().is_success() {
            let status = response.status();
            let detail = response.text().await.unwrap_or_default();
            return Err(anyhow!("Screenshot failed ({status}): {detail}"));
        }
        response
            .bytes()
            .await
            .map(|bytes| bytes.to_vec())
            .context("Failed to read screenshot bytes")
    }
}
